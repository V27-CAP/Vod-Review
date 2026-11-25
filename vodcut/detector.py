from __future__ import annotations

import logging
import math
from bisect import bisect_left, bisect_right
from pathlib import Path

import numpy as np
from scipy.ndimage import maximum_filter1d

from .core import CACHE, Progress, cache, identity, key
from .media import ffmpeg, prepare, probe
from .signals import audio_features, speech_features, visual_features

log = logging.getLogger("vodcut")


def normalize(values: np.ndarray, cfg: dict) -> np.ndarray:
    values = np.asarray(values, dtype=float)
    lo, hi = np.percentile(values, [cfg["floor_percentile"], cfg["ceiling_percentile"]])
    if hi <= lo:
        hi = values.max()
    if hi <= lo:
        return np.zeros_like(values)
    return np.clip((values - lo) / (hi - lo), 0, 1)


def near(points: list, target: float, low: float, high: float, radius: float) -> float:
    a = bisect_left(points, max(low, target - radius))
    b = bisect_right(points, min(high, target + radius))
    return min(points[a:b], key=lambda v: abs(v - target), default=target)


def bounds(peak: float, duration: float, starts: list, ends: list, c: dict) -> tuple[float, float]:
    minimum, maximum = min(duration, c["min_seconds"]), min(duration, c["max_seconds"])
    start = max(0, peak - min(c["pre_roll"], maximum * 0.8))
    end = min(duration, peak + c["post_roll"] + 1, start + maximum)
    start = near(starts, start, max(0, end - maximum), peak, c["snap_seconds"])
    end = near(ends, end, peak + 0.001, min(duration, start + maximum), c["snap_seconds"])
    if end - start < minimum:
        end = min(duration, start + minimum)
        start = max(0, end - minimum)
    return round(start, 4), round(end, 4)


def snippet(segments: list, start: float, end: float) -> str:
    parts = []
    for s in segments:
        if s["start"] >= end:
            break
        if s["end"] <= start:
            continue
        words = s.get("words", [])
        parts.append(
            "".join(w["text"] for w in words if w["end"] > start and w["start"] < end).strip()
            if words
            else s["text"]
        )
    return " ".join(p for p in parts if p)


def rank(
    signals: dict, segments: list, starts: list, ends: list, duration: float, cfg: dict
) -> tuple[list, list]:
    n = math.ceil(duration)
    c = cfg["clips"]
    active = {k: normalize(v, cfg["features"]) for k, v in signals.items() if cfg["weights"][k] > 0}
    if not active:
        raise ValueError("No enabled signals with positive weights")
    denom = sum(cfg["weights"][k] for k in active)
    scores = sum(v * cfg["weights"][k] for k, v in active.items()) / denom
    # Reward peaks with spoken setup and follow-through, including quiet verbal punchlines.
    before, after = np.zeros(n + 1), np.zeros(n + 1)
    window = c["context_window"]
    for s in segments:
        a, b = max(0, int(s["start"])), min(n, math.ceil(s["end"]))
        if a >= b:
            continue
        before[min(n, a + 1)] += 1
        before[min(n, b + window)] -= 1
        after[max(0, a - window)] += 1
        after[b] -= 1
    context = (np.cumsum(before)[:-1] > 0) & (np.cumsum(after)[:-1] > 0)
    scores = np.clip(scores + c["context_bonus"] * context * np.sqrt(scores), 0, 1)
    peaks = np.flatnonzero(
        (scores >= maximum_filter1d(scores, size=cfg["features"]["peak_window"]))
        & (scores > 0)
        & (scores >= c["min_score"])
    )
    peaks = sorted(peaks, key=lambda p: -scores[p])
    starts, ends = sorted(starts), sorted(ends)
    merge_gap = float(c.get("merge_gap", 0))
    max_seconds = min(duration, float(c["max_seconds"]))
    clips = []
    for p in peaks:
        peak = min(float(p) + 0.5, max(0, duration - 0.001))
        a, b = bounds(peak, duration, starts, ends, c)
        if merge_gap > 0:
            neighbors = [x for x in clips if a <= x["end"] + merge_gap and b >= x["start"] - merge_gap]
            if neighbors:
                target = max(neighbors, key=lambda x: x["score"])
                new_a = round(min(target["start"], a), 4)
                new_b = round(max(target["end"], b), 4)
                if new_b - new_a <= max_seconds and not any(
                    other is not target and new_a < other["end"] and new_b > other["start"] for other in clips
                ):
                    target["start"] = new_a
                    target["end"] = new_b
                    target["transcript"] = snippet(segments, new_a, new_b)
                    merged = target.get("merged_peaks", 1) + 1
                    target["merged_peaks"] = merged
                    if merged <= 4:
                        target["score"] = round(min(1.0, target["score"] + 0.025), 4)
                    continue
        if len(clips) >= c["top_n"]:
            continue
        if any(a < x["end"] and b > x["start"] for x in clips):
            continue
        parts = {k: round(float(v[p]), 4) for k, v in active.items()}
        leading = sorted(
            (k for k in parts if parts[k] > 0), key=lambda k: -parts[k] * cfg["weights"][k]
        )[:2]
        clips.append(
            dict(
                id=f"p{p}",
                name=f"Highlight {len(clips) + 1:02d}",
                start=a,
                end=b,
                peak=peak,
                score=round(float(scores[p]), 4),
                breakdown=parts,
                signals=leading,
                context=bool(context[p]),
                transcript=snippet(segments, a, b),
                status="pending",
                thumbnail=f"p{p}.jpg",
            )
        )
        if not merge_gap and len(clips) >= c["top_n"]:
            break
    if merge_gap > 0:
        clips.sort(key=lambda x: -x["score"])
    return clips, [round(float(x.max()), 3) for x in np.array_split(scores, min(360, n))]


REASONS = {
    "loudness": ("Aumento de volume", "Loudness spike"),
    "change": ("Mudança súbita de energia", "Sudden energy change"),
    "vocal": ("Possível risada ou grito", "Possible laughter or shout"),
    "transcript": ("Reação verbal", "Spoken reaction"),
    "chat_rate": ("Pico de mensagens", "Chat activity spike"),
    "chat_laugh": ("Risadas no chat", "Chat laughter"),
    "chat_users": ("Mais participantes no chat", "Unique chatter spike"),
    "motion": ("Mudança visual", "Visual change"),
    "facecam": ("Expressão / movimento na facecam", "Facecam expression / movement"),
}


def detect(
    path: Path, cfg: dict, chat: Path | None = None, progress: Progress = lambda *_: None
) -> dict:
    from .classify import classify_clip
    from .media import normalize_facecam
    from .signals import facecam_features

    path = path.resolve()
    root = CACHE / identity(path)
    progress("probe", 0)
    meta = probe(path)
    n = math.ceil(meta["duration"])
    progress("probe", 1)
    waves, segments = prepare(meta, root, cfg, progress)
    signals, starts, ends = {}, [], []
    if cfg["analysis"]["audio"] and waves:
        signals, starts, ends = audio_features(waves, n, root, cfg["features"], progress)
    if segments:
        signals["transcript"] = speech_features(segments, n, cfg["features"])
        gap = cfg["features"]["speech_gap"]
        for i, s in enumerate(segments):
            if (
                i == 0
                or s["start"] - segments[i - 1]["end"] >= gap
                or segments[i - 1]["text"].endswith((".", "?", "!", "。", "！", "？"))
            ):
                starts.append(s["start"])
                if i:
                    ends.append(segments[i - 1]["end"])
        ends.append(segments[-1]["end"])
    if cfg["analysis"]["visual"]:
        step = cfg["features"]["visual_step"]
        signals["motion"] = cache(
            root / f"motion-{step}.json", lambda: visual_features(meta, step, progress)
        )
        progress("visual", 1)
    facecam_roi = (
        normalize_facecam(cfg["analysis"].get("facecam_roi"))
        if cfg["analysis"].get("facecam")
        else None
    )
    if facecam_roi:
        fstep = int(cfg["features"].get("facecam_step", 1))
        fsig = key("facecam-v1", facecam_roi, fstep, cfg["features"]["baseline_seconds"])
        signals["facecam"] = cache(
            root / f"facecam-{fsig}.json",
            lambda: facecam_features(meta, facecam_roi, fstep, cfg["features"], progress),
        )
        progress("facecam", 1)
    if chat and cfg["analysis"]["chat"]:
        from .chat import chat_features

        progress("chat", 0)
        sig = key(
            "chat-v1",
            identity(chat),
            *[
                cfg["features"][k]
                for k in (
                    "laughter_pattern",
                    "baseline_seconds",
                    "chat_bin_seconds",
                    "chat_delay_seconds",
                )
            ],
        )
        signals.update(cache(root / f"chat-{sig}.json", lambda: chat_features(chat, n, cfg["features"])))
        progress("chat", 1)
    output_lang = cfg["analysis"].get("output_language", "en")
    preset_name = cfg["analysis"].get("preset", "short_clips")
    progress("rank", 0)
    clips, curve = rank(signals, segments, starts, ends, meta["duration"], cfg)
    for clip in clips:
        clip["reason"] = {
            lang: " + ".join(REASONS[k][i] for k in clip["signals"])
            for i, lang in enumerate(("pt", "en"))
        }
        clip.update(classify_clip(clip, lang=output_lang))
    if cfg["analysis"]["judge"]:
        from copy import deepcopy
        from .judge import JudgeUnavailable, rerank

        judge_cfg = dict(cfg["judge"])
        judge_cfg.setdefault("output_language", output_lang)
        judge_cfg.setdefault("style", preset_name)
        try:
            clips = rerank(deepcopy(clips), judge_cfg, root, progress, meta=meta, facecam=facecam_roi)
        except JudgeUnavailable:
            meta["ai_unavailable"] = True
    for i, clip in enumerate(clips):
        progress("rank", i / max(1, len(clips)))
        thumb = root / "thumbs" / clip["thumbnail"]
        if not thumb.exists():
            ffmpeg(
                [
                    "-ss",
                    min(clip["peak"], max(0, meta["duration"] - 0.5)),
                    "-i",
                    path,
                    "-map",
                    f"0:{meta['video_index']}",
                    "-frames:v",
                    1,
                    "-vf",
                    "scale=320:-2",
                ],
                thumb,
            )
    progress("rank", 1)
    safe_cfg = dict(cfg, judge={k: v for k, v in cfg.get("judge", {}).items() if k != "api_key"})
    sig = key(safe_cfg, identity(chat) if chat else None)
    if facecam_roi:
        meta["facecam"] = facecam_roi
    meta["selected_audio_track"] = int(cfg["analysis"].get("audio_track", -1))
    meta["output_language"] = output_lang
    meta["preset"] = preset_name
    return dict(project=root.name, run=sig, meta=meta, clips=clips, curve=curve, segments=segments)
