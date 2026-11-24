# Implementation plan
1. In vodcut/media.py, probe local files and cache bounded PCM chunks and faster-whisper word timestamps. Use FFmpeg for container support and CPU INT8 for the RX 570 workstation.
2. In signals.py and detector.py, extract acoustic features with librosa and NumPy, optionally sample video, rank peaks with context and expose a CLI.
3. Serve a disk-based project and a bilingual review UI on loopback with FastAPI. Persist edits and generate short browser-compatible previews on demand.
4. Use Python's XML and CSV libraries to exchange original-media edits with Premiere. Use FFmpeg for optional MP4 delivery.
5. Add chat import adapters, an opt-in local transcript judge, an evaluation CLI and an editable TOML config. Exercise the workflow with generated media and unit tests in tests/.
