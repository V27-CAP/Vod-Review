"""Streaming adapters for CSV, chat-downloader arrays, and TwitchDownloader envelopes."""

from __future__ import annotations

import csv
import math
import re
from collections.abc import Iterator
from pathlib import Path

import numpy as np
from scipy.ndimage import median_filter


def seconds(value: object) -> float:
    fields = str(value).strip().split(":")
    if len(fields) > 3 or not fields:
        raise ValueError("Expected seconds or HH:MM:SS")
    total = 0.0
    for field in fields:
        total = 60 * total + float(field)
    if not math.isfinite(total):
        raise ValueError("Invalid timestamp")
    return total


def records(path: Path) -> Iterator[dict]:
    if path.suffix.lower() == ".csv":
        with path.open(encoding="utf-8-sig", newline="") as stream:
            yield from csv.DictReader(stream)
    elif path.suffix.lower() == ".json":
        import ijson

        with path.open("rb") as stream:
            if stream.read(3) != b"\xef\xbb\xbf":
                stream.seek(0)
            offset = stream.tell()
            # Probe just the structure, then stream each message without loading the entire log.
            prefix = None
            for name, event, _ in ijson.parse(stream):
                if event == "start_array" and name in ("", "comments", "messages", "chat"):
                    prefix = (name + "." if name else "") + "item"
                    break
            if prefix is None:
                raise ValueError("Expected a message array or comments/messages/chat array in JSON")
            stream.seek(offset)
            yield from ijson.items(stream, prefix)
    else:
        raise ValueError("Chat must be a CSV or JSON file")


def unpack(row: dict) -> tuple[float, str, str]:
    stamp = next(
        (
            row[k]
            for k in (
                "content_offset_seconds",
                "time_in_seconds",
                "time_text",
                "seconds",
                "time",
                "offset",
                "timestamp",
            )
            if row.get(k) not in (None, "")
        ),
        None,
    )
    author = (
        row.get("commenter")
        or row.get("author")
        or row.get("username")
        or row.get("user")
        or row.get("chatter")
        or ""
    )
    if isinstance(author, dict):
        author = author.get("id") or author.get("name") or author.get("display_name", "")
    message = row.get("message") or row.get("text") or row.get("body") or ""
    if isinstance(message, dict):
        message = message.get("body") or " ".join(
            f.get("text", "") for f in message.get("fragments", [])
        )
    return seconds(stamp), str(author), str(message)


def chat_features(path: Path, n: int, cfg: dict) -> dict:
    size = int(cfg["chat_bin_seconds"])
    count = math.ceil(n / size)
    rate, laugh = np.zeros(count), np.zeros(count)
    users = [set() for _ in range(count)]
    pattern = re.compile(cfg["laughter_pattern"], re.IGNORECASE)
    valid = 0
    total = 0
    for row in records(path):
        total += 1
        if not isinstance(row, dict):
            continue
        try:
            stamp, author, message = unpack(row)
        except (ValueError, TypeError):
            continue
        stamp -= cfg["chat_delay_seconds"]
        if 0 <= stamp < n:
            valid += 1
            i = int(stamp) // size
            rate[i] += 1
            laugh[i] += bool(pattern.search(message))
            if author:
                users[i].add(author)
    if total and not valid:
        raise ValueError("Chat has no readable VOD-relative timestamps inside this recording")
    window = max(3, int(cfg["baseline_seconds"] / size))

    def excess(x: np.ndarray) -> list:
        above = np.maximum(0, x - median_filter(x, size=window, mode="nearest"))
        return np.repeat(above, size)[:n].tolist()

    return dict(
        chat_rate=excess(rate),
        chat_users=excess(np.array([len(u) for u in users])),
        chat_laugh=np.repeat(laugh / np.maximum(1, rate) * np.log1p(laugh), size)[:n].tolist(),
    )
