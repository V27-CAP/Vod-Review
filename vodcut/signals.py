from __future__ import annotations

import math
import re
import subprocess
import wave
from pathlib import Path

import numpy as np
from scipy.ndimage import maximum_filter1d, median_filter

from .core import Progress, cache, key


def audio_chunk(path: Path, cfg: dict) -> dict:
    """Use 25 ms frames, then retain only two measurements per second."""
    sr = cfg["sample_rate"]
    import librosa

    with wave.open(str(path), "rb") as stream:
        raw = np.frombuffer(stream.readframes(stream.getnframes()), "<i2")
    result = {"db": [], "vocal": []}
    # Analyze small batches to bound the temporary frame matrices.
    for offset in range(0, len(raw), 10 * sr):
        y = raw[offset : offset + 10 * sr].astype(np.float32) / 32768
        count = math.ceil(len(y) / sr)
        y = np.pad(y, (0, count * sr - len(y)))
        hop = sr // 40
        rms = librosa.feature.rms(y=y, frame_length=hop, hop_length=hop, center=False)[0]
        zcr = librosa.feature.zero_crossing_rate(y, frame_length=hop, hop_length=hop, center=False)[0]
        for level, crossings in zip(rms.reshape(-1, 40), zcr.reshape(-1, 40)):
            db = float(20 * np.log10(max(0.000056, np.sqrt(np.mean(level**2)))))
            pulses = np.count_nonzero((level[1:] > np.median(level)) & (level[:-1] <= np.median(level)))
            # Repeated voiced bursts approximate laughter; sustained bright energy approximates a yell.
            brightness = float(np.mean(crossings))
            voice = min(1, brightness * cfg["vocal_zcr_scale"]) * min(
                1, max(0, (db - cfg["vocal_floor_db"]) / cfg["vocal_range_db"])
            )
            result["vocal"].append(
                float(
                    voice
                    * max(
                        min(1, pulses / cfg["vocal_pulses"]),
                        min(1, np.max(level) * cfg["vocal_peak_scale"]),
                    )
                )
            )
            result["db"].append(db)
    return result


def audio_features(
    waves: list, n: int, root: Path, cfg: dict, progress: Progress
) -> tuple[dict, list, list]:
    db = np.full(n, -85.0)
    vocal = np.zeros(n)
    for i, (start, wav) in enumerate(waves):
        progress("audio", i / len(waves))
        result = cache(
            root
            / ("acoustics-" + key("v3", {k: v for k, v in cfg.items() if k.startswith("vocal_")}))
            / wav.parent.name
            / (wav.stem + ".json"),
            lambda: audio_chunk(wav, cfg),
        )
        count = min(len(result["db"]), n - start)
        for values, name in ((db, "db"), (vocal, "vocal")):
            values[start : start + count] = result[name][:count]
    onset = np.maximum(0, np.diff(db, prepend=db[0]))
    local = median_filter(db, size=int(cfg["baseline_seconds"]), mode="nearest")
    loud = np.maximum(db - local, 0)
    voice = db >= cfg["silence_db"]
    starts = np.flatnonzero(voice & ~np.r_[False, voice[:-1]]).tolist()
    ends = np.flatnonzero(~voice & np.r_[False, voice[:-1]]).tolist()
    progress("audio", 1)
    return {"loudness": loud, "change": onset, "vocal": vocal}, starts, ends


def visual_features(meta: dict, step: int, progress: Progress) -> list:
    cmd = [
        "ffmpeg",
        "-nostdin",
        "-v",
        "error",
        "-i",
        meta["path"],
        "-map",
        f"0:{meta['video_index']}",
        "-vf",
        f"fps=1/{step},scale=64:36,format=gray",
        "-f",
        "rawvideo",
        "-",
    ]
    values = np.zeros(math.ceil(meta["duration"]))
    with subprocess.Popen(cmd, stdout=subprocess.PIPE) as proc:
        prev = None
        i = 0
        while True:
            frame = proc.stdout.read(64 * 36)
            if not frame:
                break
            arr = np.frombuffer(frame, np.uint8).astype(float)
            if prev is not None and arr.size == prev.size:
                values[i : i + step] = np.mean(np.abs(arr - prev))
            prev = arr
            i += step
            progress("visual", min(1, i / len(values)))
        if proc.wait():
            raise ValueError("FFmpeg visual analysis failed")
    return values.tolist()


def facecam_features(meta: dict, roi: dict, step: int, cfg: dict, progress: Progress) -> list:
    """Measure facial movement and expression shifts inside a selected facecam region."""
    from .media import facecam_crop_expr, normalize_facecam

    box = normalize_facecam(roi)
    if not box:
        raise ValueError("Invalid facecam region")
    step = max(1, int(step))
    crop = facecam_crop_expr(meta, box)
    cmd = [
        "ffmpeg",
        "-nostdin",
        "-v",
        "error",
        "-i",
        meta["path"],
        "-map",
        f"0:{meta['video_index']}",
        "-vf",
        f"{crop},fps=1/{step},scale=64:64,format=gray",
        "-f",
        "rawvideo",
        "-",
    ]
    n = math.ceil(meta["duration"])
    motion = np.zeros(n)
    deviation = np.zeros(n)
    gradient = np.zeros(n)

    yy, xx = np.mgrid[0:64, 0:64]
    center_weight = np.exp(-(((yy - 30.0) ** 2) / (2 * 18.0**2) + ((xx - 32.0) ** 2) / (2 * 18.0**2)))
    center_weight /= np.mean(center_weight)

    with subprocess.Popen(cmd, stdout=subprocess.PIPE) as proc:
        prev = None
        neutral = None
        i = 0
        while True:
            frame = proc.stdout.read(64 * 64)
            if not frame:
                break
            if len(frame) < 64 * 64:
                break
            arr = np.frombuffer(frame, np.uint8).astype(np.float32).reshape(64, 64)
            # Normalize per-frame brightness lightly so game screen glare doesn't dominate facecam.
            mean = float(np.mean(arr))
            std = float(np.std(arr)) + 1.0
            norm = (arr - mean) / std

            if prev is not None:
                m = float(np.mean(np.abs(norm - prev) * center_weight))
                motion[i : i + step] = m
            if neutral is None:
                neutral = norm.copy()
            else:
                # Upper face (eyes/brows) + lower face (mouth/jaw) expression deviation from neutral.
                eyes = np.abs(norm[12:34, 10:54] - neutral[12:34, 10:54])
                mouth = np.abs(norm[34:56, 14:50] - neutral[34:56, 14:50])
                dev = 0.45 * float(np.mean(eyes)) + 0.55 * float(np.mean(mouth))
                deviation[i : i + step] = dev
                neutral = 0.92 * neutral + 0.08 * norm

            # Spatial edge/gradient energy in mouth & eye zones spikes on smiles, laughs, gasps.
            gy = np.abs(np.diff(norm[12:56, 10:54], axis=0))
            gx = np.abs(np.diff(norm[12:56, 10:54], axis=1))
            gradient[i : i + step] = 0.5 * (float(np.mean(gy)) + float(np.mean(gx)))

            prev = norm
            i += step
            progress("facecam", min(1, i / max(1, n)))
        if proc.wait():
            raise ValueError("FFmpeg facecam analysis failed")

    baseline = max(3, int(cfg.get("baseline_seconds", 45)))
    grad_excess = np.maximum(0.0, gradient - median_filter(gradient, size=baseline, mode="nearest"))
    dev_excess = np.maximum(0.0, deviation - median_filter(deviation, size=baseline, mode="nearest"))

    def unit_scale(x: np.ndarray) -> np.ndarray:
        peak = float(np.percentile(x, 98)) if x.size else 0.0
        if peak <= 1e-6:
            peak = float(np.max(x)) if x.size else 0.0
        return x / peak if peak > 1e-6 else np.zeros_like(x)

    combined = 0.40 * unit_scale(motion) + 0.35 * unit_scale(dev_excess) + 0.25 * unit_scale(grad_excess)
    return combined.tolist()


def speech_features(segments: list, n: int, cfg: dict) -> np.ndarray:
    values = np.zeros(n)
    pattern = re.compile(cfg["reaction_pattern"], re.IGNORECASE)
    for s in segments:
        hits = len(pattern.findall(s["text"]))
        if not hits:
            continue
        score = min(1.0, hits / cfg["reaction_hits"])
        a = min(n - 1, max(0, int(s.get("start", s["end"]))))
        b = min(n, max(a + 1, math.ceil(float(s["end"]))))
        values[a:b] = np.maximum(values[a:b], score)
    return maximum_filter1d(values, size=3)
