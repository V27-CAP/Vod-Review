"""Build the native Windows EXE, syncing updated vodcut/ app files into the embedded runtime."""
import argparse
import hashlib
import io
import os
import re
import shutil
import struct
import subprocess
import tempfile
import zipfile
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parents[1]


def zip_offset(stream, end):
    stream.seek(end - 22)
    header = stream.read(22)
    if header[:4] != b"PK\x05\x06" or header[-2:] != b"\0\0":
        raise ValueError("Expected a VODCut ZIP with no comment")
    directory_size, directory_offset = struct.unpack_from("<II", header, 12)
    return end - 22 - directory_size - directory_offset


def player_archive(stream, runtime_start, ffmpeg):
    if ffmpeg is None:
        start = zip_offset(stream, runtime_start)
        stream.seek(start)
        return io.BytesIO(stream.read(runtime_start - start))
    packed = io.BytesIO()
    with zipfile.ZipFile(ffmpeg) as src, zipfile.ZipFile(packed, "w", zipfile.ZIP_DEFLATED) as out:
        player = next(name for name in src.namelist() if name.endswith("/bin/ffplay.exe"))
        with src.open(player) as data, out.open("ffplay.exe", "w") as dest:
            shutil.copyfileobj(data, dest, 1024 * 1024)
    return packed


def collect_app_files(repo: Path):
    """Return (arcname, bytes) pairs for all repo files placed under app/ in the runtime ZIP."""
    items = []
    items.append(("app/licenses/OpenSans-LICENSE.txt", (repo / "launcher/fonts/OpenSans-LICENSE.txt").read_bytes()))
    for name in ("PLAN.md", "README.md", "requirements.txt"):
        p = repo / name
        if p.is_file():
            items.append((f"app/{name}", p.read_bytes()))
    cfg_path = repo / "config.toml"
    if cfg_path.is_file():
        cfg_text = cfg_path.read_text(encoding="utf-8")
        # Portable runtime ships the offline Whisper base model at ../models/base relative to app/.
        cfg_text = re.sub(r'^model\s*=\s*"base"\s*$', 'model = "../models/base"', cfg_text, flags=re.MULTILINE)
        items.append(("app/config.toml", cfg_text.encode("utf-8")))
    for folder in ("vodcut", "tests", "examples"):
        base = repo / folder
        if not base.is_dir():
            continue
        for file_path in sorted(base.rglob("*")):
            if not file_path.is_file():
                continue
            if "__pycache__" in file_path.parts or file_path.suffix in {".pyc", ".pyo"}:
                continue
            rel = file_path.relative_to(repo).as_posix()
            items.append((f"app/{rel}", file_path.read_bytes()))
    return items


def build_runtime_zip(source_stream, runtime_start: int, dest_zip: Path, sync_app: bool):
    if not sync_app:
        source_stream.seek(runtime_start)
        with dest_zip.open("wb") as out:
            shutil.copyfileobj(source_stream, out, 1024 * 1024)
        return
    source_stream.seek(0)
    with zipfile.ZipFile(source_stream, "r") as src, zipfile.ZipFile(
        dest_zip, "w", compression=zipfile.ZIP_DEFLATED, compresslevel=6
    ) as dst:
        for info in src.infolist():
            if info.filename.startswith("app/"):
                continue
            with src.open(info, "r") as entry_in:
                dst.writestr(info, entry_in.read())
        for arcname, content in collect_app_files(REPO_ROOT):
            dst.writestr(arcname, content)


def build(runtime, target, ffmpeg, sync_app=True):
    with runtime.open("rb") as stream, zipfile.ZipFile(stream) as archive:
        if "python/python.exe" not in archive.namelist():
            raise ValueError("Missing portable Windows runtime")
        end = runtime.stat().st_size
        start = zip_offset(stream, end)
        player = player_archive(stream, start, ffmpeg)
        with zipfile.ZipFile(player) as package, package.open("ffplay.exe") as exe:
            player_hash = hashlib.file_digest(exe, "sha256").hexdigest()
        target.parent.mkdir(parents=True, exist_ok=True)
        with tempfile.TemporaryDirectory(dir=target.parent) as temp:
            temp = Path(temp)
            runtime_zip = temp / "runtime.zip"
            build_runtime_zip(stream, start, runtime_zip, sync_app=sync_app)
            size = runtime_zip.stat().st_size
            with runtime_zip.open("rb") as rt:
                digest = hashlib.file_digest(rt, "sha256").hexdigest()
            flags = (
                f"-H=windowsgui -s -w -X main.payloadHash={digest} -X main.payloadSize={size} "
                f"-X main.playerHash={player_hash} -X main.playerSize={player.getbuffer().nbytes}"
            )
            subprocess.run(
                [
                    "go",
                    "build",
                    "-trimpath",
                    "-buildvcs=false",
                    "-ldflags",
                    flags,
                    "-o",
                    str(temp / "shell.exe"),
                    ".",
                ],
                cwd=Path(__file__).resolve().parent,
                env=os.environ | {"GOOS": "windows", "GOARCH": "amd64", "CGO_ENABLED": "0"},
                check=True,
            )
            player.seek(0)
            with (temp / "shell.exe").open("ab") as output, runtime_zip.open("rb") as rt:
                shutil.copyfileobj(player, output, 1024 * 1024)
                shutil.copyfileobj(rt, output, 1024 * 1024)
            os.replace(temp / "shell.exe", target)


if __name__ == "__main__":
    cli = argparse.ArgumentParser(description=__doc__)
    cli.add_argument("runtime", type=Path)
    cli.add_argument("output", type=Path)
    cli.add_argument("--ffmpeg", type=Path, help="Windows FFmpeg distribution ZIP for an older runtime")
    cli.add_argument(
        "--preserve-runtime",
        action="store_true",
        help="Keep the existing runtime ZIP verbatim without syncing app/ files",
    )
    args = cli.parse_args()
    build(args.runtime.resolve(), args.output.resolve(), args.ffmpeg, sync_app=not args.preserve_runtime)
