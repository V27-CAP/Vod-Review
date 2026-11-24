"""One native desktop operation. Requests and progress use anonymous pipes."""
import json
import os
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
    if action == "analyze":
        from vodcut.detector import detect
        form = request["options"]
        cfg = config()
        for name in ("transcription", "audio", "visual", "judge"):
            cfg["analysis"][name] = form[name]
        for name in ("top_n", "min_seconds", "max_seconds"):
            cfg["clips"][name] = form[name]
        project = detect(Path(form["path"]), cfg, Path(form["chat"]) if form["chat"] else None, progress)
        ref = {name: project[name] for name in ("project", "run")}
        review = CACHE / ref["project"] / f"review-{ref['run']}.json"
        if review.exists():
            edits = {clip["id"]: clip for clip in read(review)["clips"]}
            for clip in project["clips"]:
                if previous := edits.get(clip["id"]):
                    for name in ("name", "start", "end", "status", "transcript"):
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
    start, end = request["start"], request["end"]
    if not 0 <= start <= clip["peak"] < end <= meta["duration"] + .0001:
        raise ValueError("O pico precisa estar dentro do corte. / Keep the peak inside the clip.")
    if action == "edit":
        from vodcut.detector import snippet
        name = request["name"].strip()
        if not name or request["status"] not in ("pending", "approved", "rejected"):
            raise ValueError("Confira o nome e o status do corte. / Check clip name and status.")
        clip.update(name=name, start=start, end=end, status=request["status"],
                    transcript=snippet(project["segments"], start, end))
        save(review, project)
        return ""
    if action == "preview":
        from vodcut.media import ffmpeg
        if identity(Path(meta["path"])) != project_id:
            raise ValueError("O vídeo de origem mudou. / The source video changed.")
        dest = CACHE / project_id / "previews" / (key("desktop-v1", start, end) + ".mpg")
        if not dest.exists():
            progress("preview", 0)
            ffmpeg(["-ss", start, "-i", meta["path"], "-t", end - start, "-map",
                    f"0:{meta['video_index']}", "-map", "0:a:0?", "-vf",
                    "scale=-2:min(480\\,trunc(ih/2)*2)", "-r", "25", "-c:v", "mpeg1video",
                    "-q:v", "4", "-c:a", "mp2", "-ar", "44100", "-ac", "2", "-f", "mpeg"], dest)
        return str(dest)
    raise ValueError(action)


if __name__ == "__main__":
    try:
        emit(result=run(json.load(sys.stdin)))
    except Exception as exc:
        emit(error=str(exc) or type(exc).__name__)
        sys.exit(1)
