"""One native desktop operation. Requests and progress use anonymous pipes."""
import json
import sys
from pathlib import Path

from vodcut.core import CACHE, config, identity, key, read, save


def emit(**message):
    print(json.dumps(message, ensure_ascii=True), flush=True)


def run(request):
    cancel = Path(request["cancel"])
    last = None

    def progress(stage, fraction):
        nonlocal last
        if cancel.exists():
            raise InterruptedError("Cancelado / Cancelled")
        item = stage, int(fraction * 100)
        if item != last:
            emit(stage=stage, percent=item[1])
            last = item

    action = request["action"]
    if action == "probe":
        from vodcut.media import extract_frame, probe

        video_path = Path(request["path"])
        progress("probe", 0)
        meta = probe(video_path)
        frame_path = ""
        ts = 0.0
        if request.get("extract_frame", True):
            duration = float(meta["duration"])
            if request.get("timestamp") is not None:
                ts = float(request["timestamp"])
            else:
                ts = min(max(1.0, duration * 0.15), max(0.0, duration - 0.5))
            ts = min(max(0.0, ts), max(0.0, duration - 0.1))
            root = CACHE / identity(video_path)
            dest = root / "frames" / (key("frame-v1", round(ts, 1)) + ".jpg")
            if not dest.exists():
                extract_frame(meta, ts, dest)
            frame_path = str(dest)
        progress("probe", 1)
        return json.dumps({"meta": meta, "frame": frame_path, "timestamp": round(ts, 2)}, ensure_ascii=False)

    if action == "analyze":
        from vodcut.core import apply_preset
        from vodcut.detector import detect

        form = request["options"]
        cfg = config()
        if form.get("preset"):
            apply_preset(cfg, str(form["preset"]))
        if form.get("output_language") in ("en", "pt"):
            cfg["analysis"]["output_language"] = str(form["output_language"])
        for name in ("transcription", "audio", "visual", "facecam", "voice_isolation", "judge"):
            if name in form:
                cfg["analysis"][name] = bool(form[name])
        if "audio_track" in form:
            cfg["analysis"]["audio_track"] = int(form["audio_track"])
        if form.get("facecam_roi"):
            cfg["analysis"]["facecam_roi"] = form["facecam_roi"]
        if isinstance(form.get("ai"), dict):
            ai = form["ai"]
            for name in ("provider", "endpoint", "model", "api_key"):
                if name in ai and ai[name] is not None:
                    cfg["judge"][name] = str(ai[name])
            if "vision" in ai:
                cfg["judge"]["vision"] = bool(ai["vision"])
        for name in ("top_n", "min_seconds", "max_seconds"):
            if name in form and form[name] is not None:
                cfg["clips"][name] = form[name]
        project = detect(Path(form["path"]), cfg, Path(form["chat"]) if form["chat"] else None, progress)
        ref = {name: project[name] for name in ("project", "run")}
        review = CACHE / ref["project"] / f"review-{ref['run']}.json"
        if review.exists():
            edits = {clip["id"]: clip for clip in read(review)["clips"]}
            for clip in project["clips"]:
                if previous := edits.get(clip["id"]):
                    for name in (
                        "name",
                        "start",
                        "end",
                        "status",
                        "transcript",
                        "category",
                        "categories",
                        "context_note",
                        "description",
                        "hashtags",
                    ):
                        if name in previous:
                            clip[name] = previous[name]
        save(review, project)
        save(CACHE / "last.json", ref)
        return ""

    project_id, run_id = request["project"], request["run"]
    review = CACHE / project_id / f"review-{run_id}.json"
    project = read(review)
    meta = project["meta"]
    if action == "export":
        from vodcut.export import deliver

        return str(deliver(project, request["formats"], request["vertical"], progress))

    clip = next(c for c in project["clips"] if c["id"] == request["id"])
    start, end = float(request["start"]), float(request["end"])
    if not 0 <= start <= clip["peak"] < end <= meta["duration"] + 0.0001:
        raise ValueError("O pico precisa estar dentro do corte. / Keep the peak inside the clip.")
    if action == "edit":
        from vodcut.classify import normalize_category, normalize_hashtags
        from vodcut.detector import snippet

        name = request["name"].strip()
        if not name or request["status"] not in ("pending", "approved", "rejected"):
            raise ValueError("Confira o nome e o status do corte. / Check clip name and status.")
        updates = dict(
            name=name,
            start=start,
            end=end,
            status=request["status"],
            transcript=snippet(project["segments"], start, end),
        )
        if request.get("category"):
            cat = normalize_category(request["category"])
            updates["category"] = cat
            cats = [cat] + [x for x in clip.get("categories", []) if x != cat]
            updates["categories"] = cats
        if "context_note" in request and request["context_note"] is not None:
            updates["context_note"] = str(request["context_note"]).strip()
        if "description" in request and request["description"] is not None:
            updates["description"] = str(request["description"]).strip()
        if "hashtags" in request and request["hashtags"] is not None:
            updates["hashtags"] = normalize_hashtags(
                request["hashtags"], updates.get("category", clip.get("category", "funny"))
            )
        clip.update(updates)
        save(review, project)
        return ""
    if action == "preview":
        from vodcut.media import ffmpeg

        if identity(Path(meta["path"])) != project_id:
            raise ValueError("O vídeo de origem mudou. / The source video changed.")
        pstart = float(request.get("preview_start", start))
        pend = float(request.get("preview_end", end))
        pstart = max(0.0, min(pstart, start))
        pend = min(float(meta["duration"]), max(pend, end))
        if pend <= pstart + 0.1:
            pstart, pend = start, end
        dest = CACHE / project_id / "previews" / (key("desktop-v2", round(pstart, 3), round(pend, 3)) + ".mpg")
        if not dest.exists():
            progress("preview", 0)
            ffmpeg(
                [
                    "-ss",
                    pstart,
                    "-i",
                    meta["path"],
                    "-t",
                    pend - pstart,
                    "-map",
                    f"0:{meta['video_index']}",
                    "-map",
                    "0:a:0?",
                    "-vf",
                    "scale=-2:min(480\\,trunc(ih/2)*2)",
                    "-r",
                    "25",
                    "-c:v",
                    "mpeg1video",
                    "-q:v",
                    "4",
                    "-c:a",
                    "mp2",
                    "-ar",
                    "44100",
                    "-ac",
                    "2",
                    "-f",
                    "mpeg",
                ],
                dest,
            )
        return json.dumps(
            {"path": str(dest), "preview_start": pstart, "preview_end": pend},
            ensure_ascii=False,
        )
    raise ValueError(action)


if __name__ == "__main__":
    try:
        emit(result=run(json.load(sys.stdin)))
    except Exception as exc:
        emit(error=str(exc) or type(exc).__name__)
        sys.exit(1)
