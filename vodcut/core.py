from __future__ import annotations

import hashlib
import json
import logging
import math
import os
import tempfile
import tomllib
from collections.abc import Callable
from pathlib import Path
from typing import Any

ROOT = Path(__file__).resolve().parents[1]
CACHE = Path(os.environ.get("VODCUT_CACHE", Path.home() / "VODCut-data")).resolve()
Progress = Callable[[str, float], None]
log = logging.getLogger("vodcut")


def config(path: Path = ROOT / "config.toml") -> dict:
    with path.open("rb") as stream:
        cfg = tomllib.load(stream)
    model_path = path.resolve().parent / cfg["analysis"]["model"]
    if model_path.is_dir():
        cfg["analysis"]["model"] = str(model_path)
    a, f, c = cfg["analysis"], cfg["features"], cfg["clips"]
    a.setdefault("facecam", False)
    a.setdefault("audio_track", -1)
    a.setdefault("voice_isolation", True)
    f.setdefault("facecam_step", 1)
    f.setdefault("voice_side_level", 0.15)
    f.setdefault("voice_highpass_hz", 120)
    f.setdefault("voice_lowpass_hz", 3800)
    f.setdefault("voice_denoise_nf", -25)
    f.setdefault("vad_threshold", 0.5)
    f.setdefault("vad_min_speech_ms", 250)
    f.setdefault("vad_min_silence_ms", 400)
    cfg["weights"].setdefault("facecam", 0.12)
    if not (
        1 <= a["chunk_seconds"] <= 600
        and 1 <= a["cpu_threads"] <= 64
        and a["beam_size"] > 0
        and f["sample_rate"] == 16000
        and isinstance(a["audio_track"], int)
        and a["audio_track"] >= -1
    ):
        raise ValueError("Invalid audio or transcription configuration")
    if not (
        0 < c["min_seconds"] <= c["max_seconds"]
        and c["top_n"] > 0
        and 0 <= c["min_score"] <= 1
        and min(c["pre_roll"], c["post_roll"], c["snap_seconds"]) >= 0
    ):
        raise ValueError("Invalid clip settings")
    if not (
        0 <= f["floor_percentile"] < f["ceiling_percentile"] <= 100
        and f["visual_step"] >= 1
        and f["facecam_step"] >= 1
        and f["baseline_seconds"] >= 1
        and 0 <= f["voice_side_level"] <= 1
        and 20 <= f["voice_highpass_hz"] < f["voice_lowpass_hz"] <= 8000
        and 0 < f["vad_threshold"] < 1
        and f["vad_min_speech_ms"] > 0
        and f["vad_min_silence_ms"] > 0
    ):
        raise ValueError("Invalid feature configuration")
    if any(not math.isfinite(v) or v < 0 for v in cfg["weights"].values()):
        raise ValueError("Weights must be finite and nonnegative")
    if (
        min(
            f["peak_window"],
            f["chat_bin_seconds"],
            f["reaction_hits"],
            f["vocal_pulses"],
            f["vocal_range_db"],
        )
        <= 0
    ):
        raise ValueError("Signal windows and scales must be positive")
    judge = cfg["judge"]
    judge.setdefault("provider", "lmstudio")
    judge.setdefault("api_key", "")
    judge.setdefault("vision", False)
    if not (
        0 <= judge["blend"] <= 1
        and judge["understandability_weight"] >= 0
        and judge["shortlist"] > 0
        and judge["timeout"] > 0
        and 0 <= c["context_bonus"] <= 1
        and c["context_window"] > 0
    ):
        raise ValueError("Invalid judge or context configuration")
    return cfg


def key(*items: Any) -> str:
    data = json.dumps(items, sort_keys=True, ensure_ascii=False, default=str)
    return hashlib.sha256(data.encode()).hexdigest()[:24]


def identity(path: Path) -> str:
    stat = path.stat()
    return key(path.resolve(), stat.st_size, stat.st_mtime_ns)


def save(path: Path, data: Any) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    with tempfile.NamedTemporaryFile(
        mode="w", encoding="utf-8", dir=path.parent, delete=False
    ) as stream:
        temp = Path(stream.name)
        try:
            json.dump(data, stream, ensure_ascii=False, allow_nan=False)
        except Exception:
            stream.close()
            temp.unlink(missing_ok=True)
            raise
    temp.replace(path)


def read(path: Path) -> Any:
    return json.loads(path.read_text(encoding="utf-8"))


def cache(path: Path, build: Callable[[], Any]) -> Any:
    if path.exists():
        return read(path)
    value = build()
    save(path, value)
    return value
