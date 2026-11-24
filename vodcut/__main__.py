import argparse
import json
import logging
from pathlib import Path

from .core import ROOT, config


def main() -> None:
    parser = argparse.ArgumentParser(prog="vodcut")
    parser.add_argument("--config", type=Path, default=ROOT / "config.toml")
    sub = parser.add_subparsers(dest="action", required=True)
    sub.add_parser("model", help="Download the configured multilingual Whisper model")
    sub.add_parser("serve", help="Start the local review desk")
    analyze = sub.add_parser("detect")
    analyze.add_argument("vod", type=Path)
    analyze.add_argument("--chat", type=Path)
    analyze.add_argument("--no-transcription", action="store_true")
    args = parser.parse_args()
    logging.basicConfig(level=logging.INFO, format="%(asctime)s %(levelname)s %(message)s")
    cfg = config(args.config)
    if args.action == "model":
        from faster_whisper.utils import download_model

        if Path(cfg["analysis"]["model"]).is_dir():
            print(Path(cfg["analysis"]["model"]).resolve())
        else:
            print(download_model(cfg["analysis"]["model"]))
    elif args.action == "serve":
        from .server import serve

        serve(args.config)
    else:
        from .detector import detect

        if args.no_transcription:
            cfg["analysis"]["transcription"] = False
        project = detect(args.vod, cfg, args.chat, lambda s, p: logging.info("%s %.0f%%", s, p * 100))
        print(json.dumps(project["clips"], indent=2, ensure_ascii=False))


if __name__ == "__main__":
    main()
