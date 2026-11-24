from __future__ import annotations

import json
import math
import subprocess
import uuid
import wave
from collections.abc import Iterator
from fractions import Fraction
from pathlib import Path

import numpy as np

from .core import Progress, cache, key, log


def ffmpeg(args: list, target: Path) -> None:
    target.parent.mkdir(parents=True, exist_ok=True)
    temp = target.with_name(uuid.uuid4().hex + target.suffix)
    try:
        run = subprocess.run(
            ["ffmpeg", "-nostdin", "-v", "error", "-y", "-filter_threads", "2",
             "-threads", "2", *map(str, args), "-threads", "2", str(temp)], capture_output=True
        )
        if run.returncode:
            raise ValueError(run.stderr.decode(errors="replace")[-1800:] or "FFmpeg failed")
        temp.replace(target)
    finally:
        temp.unlink(missing_ok=True)


def probe(path: Path) -> dict:
    if path.suffix.lower() not in {".mp4", ".mov", ".mkv"} or not path.is_file():
        raise ValueError("Select an existing MP4, MKV or MOV file.")
    run = subprocess.run(
        ["ffprobe", "-v", "error", "-of", "json", "-show_format", "-show_streams", str(path)],
        capture_output=True,
        check=True,
    )
    data = json.loads(run.stdout)
    video = next(
        (
            s
            for s in data["streams"]
            if s["codec_type"] == "video" and not s.get("disposition", {}).get("attached_pic")
        ),
        None,
    )
    if video is None:
        raise ValueError("No video stream")
    audio = next((s for s in data["streams"] if s["codec_type"] == "audio"), {})
    duration = float(data["format"]["duration"])
    fps = video["avg_frame_rate"]
    if fps in {"0/0", "0/1"}:
        fps = video["r_frame_rate"]
    if not math.isfinite(duration) or duration <= 0 or Fraction(fps) <= 0:
        raise ValueError("Invalid source timing")
    return dict(
        path=str(path.resolve()),
        duration=duration,
        fps=fps,
        width=video["width"],
        height=video["height"],
        video_index=video["index"],
        audio=bool(audio),
        channels=audio.get("channels", 0),
        audio_rate=int(audio.get("sample_rate", 48000)),
        sar=video.get("sample_aspect_ratio", "1:1"),
        field=video.get("field_order", "progressive"),
        timecode=video.get("tags", {}).get(
            "timecode", data["format"].get("tags", {}).get("timecode", "")
        ),
    )


def chunks(duration: float, size: int) -> Iterator[tuple[int, float]]:
    for start in range(0, math.ceil(duration), size):
        yield start, min(size, duration - start)


def prepare(meta: dict, root: Path, cfg: dict, progress: Progress) -> tuple[list, list]:
    settings = cfg["analysis"]
    if not meta["audio"] or not (settings["transcription"] or settings["audio"]):
        return [], []
    size = settings["chunk_seconds"]
    waves, transcript = [], []
    model = None
    timing = list(chunks(meta["duration"], size))
    for i, (start, length) in enumerate(timing):
        wav = root / f"pcm-{size}" / f"{start}.wav"
        progress("extract", i / len(timing))
        if not wav.exists():
            ffmpeg(
                [
                    "-ss",
                    start,
                    "-i",
                    meta["path"],
                    "-t",
                    length,
                    "-map",
                    "0:a:0",
                    "-vn",
                    "-af",
                    "aresample=async=1:first_pts=0",
                    "-ac",
                    1,
                    "-ar",
                    16000,
                    "-c:a",
                    "pcm_s16le",
                ],
                wav,
            )
        waves.append((start, wav))
    progress("extract", 1)
    if settings["transcription"]:
        model_id = Path(settings["model"])
        signature = key(
            settings["model"],
            model_id.resolve().as_posix() if model_id.is_dir() else "",
            (model_id / "model.bin").stat().st_mtime_ns if (model_id / "model.bin").is_file() else 0,
            settings["language"],
            size,
            settings["beam_size"],
            "words-v1",
        )
        for i, (start, wav) in enumerate(waves):
            progress("transcribe", i / len(waves))

            def transcribe() -> list:
                nonlocal model
                if model is None:
                    from faster_whisper import WhisperModel
                    from faster_whisper.utils import download_model

                    try:
                        local = (
                            model_id
                            if model_id.is_dir()
                            else Path(download_model(settings["model"], local_files_only=True))
                        )
                        if not (local / "tokenizer.json").is_file():
                            raise ValueError("The local model directory needs tokenizer.json")
                        model = WhisperModel(
                            str(local),
                            device="cpu",
                            compute_type="int8",
                            cpu_threads=settings["cpu_threads"],
                            local_files_only=True,
                        )
                    except (OSError, ValueError) as exc:
                        raise ValueError(
                            "Whisper model unavailable. Run python -m vodcut model, "
                            "or disable transcription."
                        ) from exc
                language = None if settings["language"] == "auto" else settings["language"]
                with wave.open(str(wav), "rb") as stream:
                    samples = (
                        np.frombuffer(stream.readframes(stream.getnframes()), "<i2").astype(np.float32)
                        / 32768
                    )
                if not samples.size:
                    return []
                segments, info = model.transcribe(
                    samples,
                    language=language,
                    word_timestamps=True,
                    vad_filter=True,
                    beam_size=settings["beam_size"],
                )
                log.info("Transcribing chunk %s in %s", start, info.language)
                return [
                    dict(
                        start=start + s.start,
                        end=min(meta["duration"], start + s.end),
                        text=s.text.strip(),
                        language=info.language,
                        words=[
                            dict(start=start + w.start, end=start + w.end, text=w.word)
                            for w in s.words or []
                        ],
                    )
                    for s in segments
                    if s.text.strip()
                ]

            transcript.extend(cache(root / f"words-{signature}" / f"{start}.json", transcribe))
        progress("transcribe", 1)
    return waves, transcript
