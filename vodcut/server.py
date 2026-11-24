from __future__ import annotations

import copy
import socket
import urllib.request
import logging
import os
import subprocess
import sys
import threading
import webbrowser
from pathlib import Path
from typing import Literal

import uvicorn
from fastapi import FastAPI, HTTPException, Request
from fastapi.responses import FileResponse, JSONResponse
from fastapi.staticfiles import StaticFiles
from pydantic import BaseModel, Field
from starlette.middleware.trustedhost import TrustedHostMiddleware

from .core import CACHE, ROOT, config, identity, key, read, save
from .detector import detect, snippet
from .media import ffmpeg

WEB = Path(__file__).parent / "web"


class Analyze(BaseModel):
    path: str
    chat: str = ""
    transcription: bool = True
    audio: bool = True
    visual: bool = False
    judge: bool = False
    top_n: int = Field(default=20, ge=1, le=250)
    min_seconds: int = Field(default=15, ge=1, le=900)
    max_seconds: int = Field(default=90, ge=1, le=900)


class Edit(BaseModel):
    project: str
    run: str
    name: str = Field(min_length=1, max_length=160)
    start: float
    end: float
    status: Literal["approved", "pending", "rejected"]


class Export(BaseModel):
    project: str
    run: str
    formats: list[Literal["xml", "csv", "json", "mp4"]]
    vertical: bool = False


class Desk:
    def __init__(self, cfg_path: Path):
        self.cfg_path = cfg_path
        self.lock = threading.RLock()
        self.project = None
        self.job = {"busy": False, "steps": {}, "error": "", "result": ""}
        self.cancelled = False
        self.current = CACHE / "last.json"
        if self.current.exists():
            p = read(self.current)
            self.project = read(self.file(p)) if self.file(p).exists() else None

    @staticmethod
    def file(p: dict) -> Path:
        return CACHE / p["project"] / f"review-{p['run']}.json"

    def persist(self) -> None:
        save(self.file(self.project), self.project)
        save(self.current, {k: self.project[k] for k in ("project", "run")})

    def begin(self, work) -> None:
        with self.lock:
            if self.job["busy"]:
                raise HTTPException(409, "A job is already running")
            self.job = {"busy": True, "steps": {}, "error": "", "result": ""}
            self.cancelled = False

        def worker():
            try:
                work()
            except Exception as exc:
                logging.exception("Job stopped")
                with self.lock:
                    self.job["error"] = str(exc)
            finally:
                with self.lock:
                    self.job["busy"] = False

        threading.Thread(target=worker, daemon=True).start()

    def progress(self, stage: str, part: float) -> None:
        with self.lock:
            if self.cancelled:
                raise ValueError("Cancelled / Cancelado")
            if stage not in self.job["steps"]:
                logging.getLogger("vodcut").info("Starting %s", stage)
            self.job["steps"][stage] = part

    def analyze(self, form: Analyze) -> None:
        cfg = config(self.cfg_path)
        cfg["analysis"].update(
            {k: getattr(form, k) for k in ("transcription", "audio", "visual", "judge")}
        )
        cfg["clips"].update({k: getattr(form, k) for k in ("top_n", "min_seconds", "max_seconds")})
        p = detect(Path(form.path), cfg, Path(form.chat) if form.chat else None, self.progress)
        with self.lock:
            old = self.file(p)
            if old.exists():
                edits = {c["id"]: c for c in read(old)["clips"]}
                for c in p["clips"]:
                    previous = edits.get(c["id"])
                    if previous:
                        for k in ("name", "start", "end", "status", "transcript"):
                            c[k] = previous[k]
            self.project = p
            self.persist()

    def require(self) -> dict:
        if not self.project:
            raise HTTPException(404, "No active VOD")
        return self.project

    def clip(self, cid: str) -> dict:
        for c in self.require()["clips"]:
            if c["id"] == cid:
                return c
        raise HTTPException(404, "Clip not found")


def create_app(cfg_path: Path = ROOT / "config.toml") -> FastAPI:
    app = FastAPI(docs_url=None, redoc_url=None, openapi_url=None)
    app.add_middleware(TrustedHostMiddleware, allowed_hosts=["127.0.0.1", "localhost", "testserver"])
    desk = Desk(cfg_path)

    @app.middleware("http")
    async def local_api(request: Request, call_next):
        if request.method not in {"HEAD", "GET"} and request.headers.get("x-vodcut") != "local":
            return JSONResponse({"detail": "Local app request required"}, status_code=403)
        return await call_next(request)

    @app.get("/api/product")
    def product():
        return {"product": "VODCut Studio"}

    @app.get("/api/state")
    def state():
        with desk.lock:
            p = {k: v for k, v in desk.project.items() if k != "segments"} if desk.project else None
            cfg = config(cfg_path)
            return copy.deepcopy(dict(job=desk.job, project=p, defaults=cfg["analysis"] | cfg["clips"]))

    @app.post("/api/pick/{kind}")
    def pick(kind: Literal["video", "chat"]):
        if sys.platform == "win32":
            from .winpicker import pick_file
            return {"path": pick_file(kind)}
        extensions = "*.mp4 *.mkv *.mov" if kind == "video" else "*.csv *.json"
        code = (
            "import json,tkinter as tk; from tkinter import filedialog; "
            "r=tk.Tk(); r.withdraw(); r.attributes('-topmost',True); "
            f"print(json.dumps(filedialog.askopenfilename(parent=r,filetypes=[('Files','{extensions}')])))"
        )
        run = subprocess.run([sys.executable, "-c", code], capture_output=True)
        if run.returncode:
            raise HTTPException(400, "Use the file path field / Use o campo de caminho do arquivo")
        import json

        return {"path": json.loads(run.stdout)}

    @app.post("/api/analyze")
    def analyze(form: Analyze):
        if form.max_seconds < form.min_seconds:
            raise HTTPException(422, "Invalid clip duration")
        desk.begin(lambda: desk.analyze(form))
        return {"started": True}

    @app.post("/api/cancel")
    def cancel():
        with desk.lock:
            desk.cancelled = True
        return {"ok": True}

    @app.post("/api/clip/{cid}")
    def edit(cid: str, form: Edit):
        with desk.lock:
            p = desk.require()
            if desk.job["busy"] or form.project != p["project"] or form.run != p["run"]:
                raise HTTPException(409, "Refresh this project")
            c = desk.clip(cid)
            if (
                not 0 <= form.start <= c["peak"] < form.end <= p["meta"]["duration"] + 0.0001
                or not form.name.strip()
            ):
                raise HTTPException(
                    422, "Keep the peak inside the clip / Mantenha o pico dentro do corte"
                )
            c.update(
                name=form.name.strip(),
                start=form.start,
                end=form.end,
                status=form.status,
                transcript=snippet(p["segments"], form.start, form.end),
            )
            desk.persist()
            return dict(c)

    @app.get("/api/media/{pid}/{cid}/{kind}")
    def media(pid: str, cid: str, kind: Literal["thumb", "preview"]):
        with desk.lock:
            p = desk.require()
            if p["project"] != pid:
                raise HTTPException(404, "Inactive project")
            c = dict(desk.clip(cid))
            meta = p["meta"]
        root = CACHE / pid
        if kind == "thumb":
            return FileResponse(root / "thumbs" / c["thumbnail"])
        target = root / "previews" / (key(c["start"], c["end"]) + ".mp4")
        if not target.exists():
            if identity(Path(meta["path"])) != pid:
                raise HTTPException(409, "Source changed. Analyze the VOD again.")
            ffmpeg(
                [
                    "-ss",
                    c["start"],
                    "-i",
                    meta["path"],
                    "-t",
                    c["end"] - c["start"],
                    "-map",
                    f"0:{meta['video_index']}",
                    "-map",
                    "0:a:0?",
                    "-vf",
                    "scale=-2:min(720\\,trunc(ih/2)*2)",
                    "-c:v",
                    "libx264",
                    "-preset",
                    "ultrafast",
                    "-pix_fmt",
                    "yuv420p",
                    "-c:a",
                    "aac",
                    "-aac_coder",
                    "fast",
                    "-movflags",
                    "+faststart",
                ],
                target,
            )
        return FileResponse(target, media_type="video/mp4")

    @app.post("/api/export")
    def export(form: Export):
        from .export import deliver

        with desk.lock:
            p = desk.require()
            if form.project != p["project"] or form.run != p["run"]:
                raise HTTPException(409, "Refresh this project")
            if not form.formats or not any(c["status"] == "approved" for c in p["clips"]):
                raise HTTPException(422, "Select at least one approved clip and format")

            def work():
                dest = deliver(p, form.formats, form.vertical, desk.progress)
                with desk.lock:
                    desk.job["result"] = str(dest)

            desk.begin(work)
        return {"started": True}

    @app.post("/api/open-export")
    def open_export():
        path = desk.job["result"]
        if not path or not Path(path).is_dir():
            raise HTTPException(404, "No export")
        if sys.platform == "win32":
            os.startfile(path)
        else:
            subprocess.Popen(["open" if sys.platform == "darwin" else "xdg-open", path])
        return {"ok": True}

    app.mount("/", StaticFiles(directory=WEB, html=True), name="desk")
    return app


def serve(path: Path) -> None:
    import json

    port = int(os.environ.get("VODCUT_PORT", "8765"))
    url = f"http://127.0.0.1:{port}"
    with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as listener:
        if sys.platform == "win32":
            listener.setsockopt(socket.SOL_SOCKET, socket.SO_EXCLUSIVEADDRUSE, 1)
        try:
            listener.bind(("127.0.0.1", port))
        except OSError as bind_error:
            try:
                with urllib.request.urlopen(url + "/api/product", timeout=2) as response:
                    running = json.load(response).get("product") == "VODCut Studio"
            except (OSError, ValueError):
                running = False
            if running:
                if not os.environ.get("VODCUT_NO_BROWSER"):
                    webbrowser.open(url)
                return
            raise RuntimeError(f"A porta local {port} está ocupada por outro programa.") from bind_error
        server = uvicorn.Server(uvicorn.Config(create_app(path), host="127.0.0.1", port=port))
        if not os.environ.get("VODCUT_NO_BROWSER"):
            threading.Timer(1.4, lambda: webbrowser.open(url)).start()
        print(url, flush=True)
        server.run(sockets=[listener])
