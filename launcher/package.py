"""Build the native Windows EXE while preserving an earlier analysis runtime."""
import argparse
import hashlib
import io
import os
import shutil
import struct
import subprocess
import tempfile
import zipfile
from pathlib import Path


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


def build(runtime, target, ffmpeg):
    with runtime.open("rb") as stream, zipfile.ZipFile(stream) as archive:
        if "python/python.exe" not in archive.namelist():
            raise ValueError("Missing portable Windows runtime")
        end = runtime.stat().st_size
        start = zip_offset(stream, end)
        size = end - start
        player = player_archive(stream, start, ffmpeg)
        with zipfile.ZipFile(player) as package, package.open("ffplay.exe") as exe:
            player_hash = hashlib.file_digest(exe, "sha256").hexdigest()
        stream.seek(start)
        digest = hashlib.file_digest(stream, "sha256").hexdigest()
        target.parent.mkdir(parents=True, exist_ok=True)
        with tempfile.TemporaryDirectory(dir=target.parent) as temp:
            temp = Path(temp)
            flags = (f"-H=windowsgui -s -w -X main.payloadHash={digest} -X main.payloadSize={size} "
                     f"-X main.playerHash={player_hash} -X main.playerSize={player.getbuffer().nbytes}")
            subprocess.run(["go", "build", "-trimpath", "-buildvcs=false", "-ldflags", flags,
                            "-o", str(temp / "shell.exe"), "."], cwd=Path(__file__).resolve().parent,
                           env=os.environ | {"GOOS": "windows", "GOARCH": "amd64", "CGO_ENABLED": "0"},
                           check=True)
            stream.seek(start)
            player.seek(0)
            with (temp / "shell.exe").open("ab") as output:
                shutil.copyfileobj(player, output, 1024 * 1024)
                shutil.copyfileobj(stream, output, 1024 * 1024)
            os.replace(temp / "shell.exe", target)


if __name__ == "__main__":
    cli = argparse.ArgumentParser(description=__doc__)
    cli.add_argument("runtime", type=Path)
    cli.add_argument("output", type=Path)
    cli.add_argument("--ffmpeg", type=Path, help="Windows FFmpeg distribution ZIP for an older runtime")
    args = cli.parse_args()
    build(args.runtime.resolve(), args.output.resolve(), args.ffmpeg)
