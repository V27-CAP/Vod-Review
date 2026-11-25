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
    audio_streams = [s for s in data["streams"] if s["codec_type"] == "audio"]
    audio = audio_streams[0] if audio_streams else {}
    audio_tracks = [
        dict(
            stream_index=s["index"],
            audio_index=idx,
            channels=int(s.get("channels", 0) or 0),
            sample_rate=int(s.get("sample_rate", 48000) or 48000),
            codec=s.get("codec_name", ""),
            title=s.get("tags", {}).get("title", ""),
            language=s.get("tags", {}).get("language", ""),
        )
        for idx, s in enumerate(audio_streams)
    ]
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
        audio_tracks=audio_tracks,
        sar=video.get("sample_aspect_ratio", "1:1"),
        field=video.get("field_order", "progressive"),
        timecode=video.get("tags", {}).get(
            "timecode", data["format"].get("tags", {}).get("timecode", "")
        ),
    )


def select_audio_track(meta: dict, track_index: int = -1) -> tuple[str, int, bool]:
    """Return (ffmpeg_map, channels, is_dedicated_mic_track) for the chosen OBS audio track."""
    tracks = meta.get("audio_tracks") or []
    if tracks:
        if 0 <= track_index < len(tracks):
            chosen = tracks[track_index]
            dedicated = len(tracks) > 1
            return f"0:a:{chosen['audio_index']}", int(chosen.get("channels", meta.get("channels", 2))), dedicated
        first = tracks[0]
        return f"0:a:{first['audio_index']}", int(first.get("channels", meta.get("channels", 2))), False
    return "0:a:0", int(meta.get("channels", 2) or 2), False


def voice_filter_chain(
    channels: int,
    isolate: bool,
    features_cfg: dict | None = None,
    dedicated_mic: bool = False,
) -> str:
    """Build lightweight FFmpeg filter chain to isolate streamer speech from game audio."""
    f = features_cfg or {}
    parts: list[str] = []
    if isolate:
        side_level = float(f.get("voice_side_level", 0.15))
        hp = int(f.get("voice_highpass_hz", 120))
        lp = int(f.get("voice_lowpass_hz", 3800))
        nf = int(f.get("voice_denoise_nf", -25))
        if channels >= 2 and not dedicated_mic:
            parts.append(f"stereotools=mlev=1:slev={side_level:.3f}")
        parts.append(f"highpass=f={hp}")
        parts.append(f"lowpass=f={lp}")
        parts.append(f"afftdn=nf={nf}")
    parts.append("aresample=async=1:first_pts=0")
    return ",".join(parts)


def normalize_facecam(roi: dict | None) -> dict | None:
    """Validate and clamp a normalized [0..1] facecam bounding box."""
    if not isinstance(roi, dict):
        return None
    try:
        x = float(roi["x"])
        y = float(roi["y"])
        w = float(roi["w"])
        h = float(roi["h"])
    except (KeyError, TypeError, ValueError):
        return None
    if not all(math.isfinite(v) for v in (x, y, w, h)):
        return None
    x = min(max(0.0, x), 0.98)
    y = min(max(0.0, y), 0.98)
    w = min(max(0.0, w), 1.0 - x)
    h = min(max(0.0, h), 1.0 - y)
    if w < 0.02 or h < 0.02:
        return None
    return dict(x=round(x, 4), y=round(y, 4), w=round(w, 4), h=round(h, 4))


def facecam_crop_expr(meta: dict, roi: dict) -> str:
    """Compute an FFmpeg even-dimension crop expression for a normalized facecam ROI."""
    width = max(2, int(meta["width"]))
    height = max(2, int(meta["height"]))
    x = max(0, min(width - 2, int(round(roi["x"] * width)) & ~1))
    y = max(0, min(height - 2, int(round(roi["y"] * height)) & ~1))
    w = max(2, min(width - x, int(round(roi["w"] * width)) & ~1))
    h = max(2, min(height - y, int(round(roi["h"] * height)) & ~1))
    return f"crop={w}:{h}:{x}:{y}"


def extract_frame(meta: dict, timestamp: float, target: Path, max_width: int = 960) -> Path:
    """Extract a single JPEG frame from the video for visual facecam selection."""
    ts = min(max(0.0, float(timestamp)), max(0.0, float(meta["duration"]) - 0.1))
    ffmpeg(
        [
            "-ss",
            ts,
            "-i",
            meta["path"],
            "-map",
            f"0:{meta['video_index']}",
            "-frames:v",
            1,
            "-vf",
            f"scale=min({max_width}\\,iw):-2",
            "-q:v",
            3,
        ],
        target,
    )
    return target


def extract_facecam_crop(
    meta: dict, timestamp: float, roi: dict, target: Path, width: int = 384
) -> Path:
    """Extract a cropped JPEG of the facecam region at a clip peak for vision AI models."""
    box = normalize_facecam(roi)
    if not box:
        raise ValueError("Invalid facecam ROI")
    ts = min(max(0.0, float(timestamp)), max(0.0, float(meta["duration"]) - 0.1))
    crop = facecam_crop_expr(meta, box)
    ffmpeg(
        [
            "-ss",
            ts,
            "-i",
            meta["path"],
            "-map",
            f"0:{meta['video_index']}",
            "-frames:v",
            1,
            "-vf",
            f"{crop},scale=min({width}\\,iw):-2",
            "-q:v",
            3,
        ],
        target,
    )
    return target


def chunks(duration: float, size: int) -> Iterator[tuple[int, float]]:
    for start in range(0, math.ceil(duration), size):
        yield start, min(size, duration - start)


def prepare(meta: dict, root: Path, cfg: dict, progress: Progress) -> tuple[list, list]:
    settings = cfg["analysis"]
    features = cfg.get("features", {})
    if not meta["audio"] or not (settings["transcription"] or settings["audio"]):
        return [], []
    size = settings["chunk_seconds"]
    track_index = int(settings.get("audio_track", -1))
    isolate = bool(settings.get("voice_isolation", True))
    audio_map, channels, dedicated_mic = select_audio_track(meta, track_index)
    afilter = voice_filter_chain(channels, isolate, features, dedicated_mic=dedicated_mic)
    pcm_dir = root / f"pcm-{size}-{key('pcm-v2', audio_map, afilter)}"

    waves, transcript = [], []
    model = None
    timing = list(chunks(meta["duration"], size))
    for i, (start, length) in enumerate(timing):
        wav = pcm_dir / f"{start}.wav"
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
                    audio_map,
                    "-vn",
                    "-af",
                    afilter,
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
        vad_params = dict(
            threshold=float(features.get("vad_threshold", 0.5)),
            min_speech_duration_ms=int(features.get("vad_min_speech_ms", 250)),
            min_silence_duration_ms=int(features.get("vad_min_silence_ms", 400)),
            speech_pad_ms=200,
        )
        signature = key(
            settings["model"],
            model_id.resolve().as_posix() if model_id.is_dir() else "",
            (model_id / "model.bin").stat().st_mtime_ns if (model_id / "model.bin").is_file() else 0,
            settings["language"],
            size,
            settings["beam_size"],
            audio_map,
            afilter,
            vad_params,
            "words-v2",
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
                    vad_parameters=vad_params,
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
