from __future__ import annotations

import csv
import re
import shutil
import tempfile
import uuid
import xml.etree.ElementTree as ET
from datetime import datetime
from fractions import Fraction
from pathlib import Path

from .core import CACHE, Progress, identity, save
from .media import ffmpeg


def tag(parent: ET.Element, name: str, value=None, **attrs: str) -> ET.Element:
    element = ET.SubElement(parent, name, attrs)
    if value is not None:
        element.text = str(value)
    return element


def rate(parent: ET.Element, base: int, ntsc: bool) -> None:
    node = tag(parent, "rate")
    tag(node, "timebase", base)
    tag(node, "ntsc", str(ntsc).upper())


def exchange_rate(fps: str) -> tuple[int, bool, float]:
    actual = float(Fraction(fps))
    base = max(1, round(actual))
    ntsc = abs(actual - base * 1000 / 1001) < abs(actual - base)
    effective = base * 1000 / 1001 if ntsc else base
    return base, ntsc, effective


def timecode(parent: ET.Element, m: dict, base: int, ntsc: bool, source: bool) -> None:
    code = m.get("timecode", "") if source else ""
    if not re.fullmatch(r"\d\d:\d\d:\d\d[:;]\d\d", code):
        code = "00:00:00:00"
    h, minute, s, fr = map(int, re.split("[:;]", code))
    minutes = h * 60 + minute
    drop = ";" in code and ntsc and base in (30, 60)
    frame = ((minutes * 60 + s) * base + fr) - (
        round(base * 0.066667) * (minutes - minutes // 10) if drop else 0
    )
    tc = tag(parent, "timecode")
    rate(tc, base, ntsc)
    tag(tc, "string", code)
    tag(tc, "frame", frame)
    tag(tc, "displayformat", "DF" if drop else "NDF")


def video_format(parent: ET.Element, m: dict, base: int, ntsc: bool, vertical: bool) -> None:
    sample = tag(parent, "samplecharacteristics")
    rate(sample, base, ntsc)
    tag(sample, "width", 1080 if vertical else m["width"])
    tag(sample, "height", 1920 if vertical else m["height"])
    sar = m.get("sar", "1:1")
    pixels = {
        "1:1": "square",
        "8:9": "NTSC-601",
        "32:27": "NTSC-601 (16:9)",
        "16:15": "PAL-601",
        "64:45": "PAL-601 (16:9)",
    }
    tag(sample, "pixelaspectratio", "square" if vertical else pixels.get(sar, "square"))
    tag(
        sample,
        "fielddominance",
        {"tt": "upper", "tb": "upper", "bb": "lower", "bt": "lower"}.get(m.get("field"), "none"),
    )


def xml_document(m: dict, clips: list, vertical: bool) -> ET.Element:
    path = Path(m["path"]).resolve()
    base, ntsc, fps = exchange_rate(m["fps"])
    duration = max(1, round(m["duration"] * fps))
    root = ET.Element("xmeml", version="4")
    seq = tag(root, "sequence", id="sequence-1")
    tag(seq, "name", path.stem + " — VODCut")
    total = tag(seq, "duration")
    rate(seq, base, ntsc)
    timecode(seq, m, base, ntsc, False)
    media = tag(seq, "media")
    video = tag(media, "video")
    video_format(tag(video, "format"), m, base, ntsc, vertical)
    tracks = [tag(video, "track")]
    if m["channels"]:
        audio = tag(media, "audio")
        tag(audio, "numOutputChannels", max(2, m["channels"]))
        sample = tag(tag(audio, "format"), "samplecharacteristics")
        tag(sample, "depth", 16)
        tag(sample, "samplerate", m["audio_rate"])
        tracks.extend(tag(audio, "track") for _ in range(m["channels"]))
    cursor = 0
    for i, c in enumerate(clips, 1):
        a = min(duration - 1, max(0, round(c["start"] * fps)))
        b = min(duration, max(a + 1, round(c["end"] * fps)))
        length = b - a
        ids = [f"clip-{i}-{j}" for j in range(len(tracks))]
        for j, track in enumerate(tracks):
            item = tag(track, "clipitem", id=ids[j])
            tag(item, "name", c["name"])
            tag(item, "enabled", "TRUE")
            tag(item, "duration", duration)
            rate(item, base, ntsc)
            for name, val in (("start", cursor), ("end", cursor + length), ("in", a), ("out", b)):
                tag(item, name, val)
            file = tag(item, "file", id="source-1")
            if i == 1 and j == 0:
                tag(file, "name", path.name)
                tag(file, "pathurl", path.as_uri())
                rate(file, base, ntsc)
                tag(file, "duration", duration)
                timecode(file, m, base, ntsc, True)
                fmedia = tag(file, "media")
                video_format(tag(fmedia, "video"), m, base, ntsc, False)
                if m["channels"]:
                    fa = tag(fmedia, "audio")
                    fs = tag(fa, "samplecharacteristics")
                    tag(fs, "depth", 16)
                    tag(fs, "samplerate", m["audio_rate"])
                    tag(fa, "channelcount", m["channels"])
            if j:
                st = tag(item, "sourcetrack")
                tag(st, "mediatype", "audio")
                tag(st, "trackindex", j)
            for k, linked in enumerate(ids):
                link = tag(item, "link")
                tag(link, "linkclipref", linked)
                tag(link, "mediatype", "audio" if k else "video")
                tag(link, "trackindex", max(1, k))
                tag(link, "clipindex", i)
        marker = tag(seq, "marker")
        tag(marker, "name", c["name"])
        tag(marker, "comment", "Highlight peak")
        tag(marker, "in", cursor + min(length - 1, max(0, round(c["peak"] * fps) - a)))
        tag(marker, "out", -1)
        cursor += length
    total.text = str(cursor)
    return root


def validate_xml(path: Path) -> None:
    root = ET.parse(path).getroot()
    seq = root.find("sequence")
    if root.tag != "xmeml" or root.get("version") != "4" or seq is None:
        raise ValueError("Invalid FCP 7 document")
    items = seq.findall(".//clipitem")
    ids = {x.get("id") for x in items}
    if (
        len(ids) != len(items)
        or not items
        or not seq.findtext(".//file/pathurl", "").startswith("file:")
    ):
        raise ValueError("Invalid media references")
    for track in seq.findall("./media/*/track"):
        cursor = 0
        for item in track.findall("clipitem"):
            a, b, start, end, duration = (
                int(item.findtext(k)) for k in ("in", "out", "start", "end", "duration")
            )
            if not (0 <= a < b <= duration and start == cursor and end - start == b - a):
                raise ValueError("Invalid source or sequence frame range")
            if any(link.text not in ids for link in item.findall("./link/linkclipref")):
                raise ValueError("Broken clip link")
            cursor = end
        if cursor != int(seq.findtext("duration")):
            raise ValueError("Sequence duration mismatch")
    markers = seq.findall("marker")
    video = seq.findall("./media/video/track/clipitem")
    if len(markers) != len(video) or any(
        not int(c.findtext("start")) <= int(m.findtext("in")) < int(c.findtext("end"))
        for c, m in zip(video, markers)
    ):
        raise ValueError("Invalid peak markers")


def write_xml(path: Path, meta: dict, clips: list, vertical: bool) -> None:
    root = xml_document(meta, clips, vertical)
    ET.indent(root)
    path.write_bytes(
        b'<?xml version="1.0" encoding="UTF-8"?>\n<!DOCTYPE xmeml>\n'
        + ET.tostring(root, encoding="utf-8")
    )
    validate_xml(path)


def _clock(seconds: float) -> str:
    total = max(0, int(round(seconds)))
    return f"{total // 3600:02d}:{(total // 60) % 60:02d}:{total % 60:02d}"


def build_posts_text(clips: list) -> str:
    from .classify import category_label, format_post

    sections: list[str] = []
    for i, c in enumerate(clips, 1):
        cat = category_label(c.get("category", "funny"), "pt")
        header = f"=== {i:02d}. {c['name']} [{cat}] ({_clock(c['start'])} - {_clock(c['end'])}) ==="
        sections.append(header + "\n" + format_post(c, "pt"))
    return "\n\n\n".join(sections) + ("\n" if sections else "")


def deliver(project: dict, formats: list, vertical: bool, progress: Progress) -> Path:
    from .classify import category_label, format_post, normalize_category, normalize_hashtags

    clips = sorted((c for c in project["clips"] if c["status"] == "approved"), key=lambda c: c["start"])
    if not clips:
        raise ValueError("No approved clips")
    meta = project["meta"]
    if identity(Path(meta["path"])) != project["project"]:
        raise ValueError("Source changed. Analyze the VOD again.")
    shelf = CACHE / "exports"
    shelf.mkdir(parents=True, exist_ok=True)
    temp = Path(tempfile.mkdtemp(dir=shelf, prefix=".partial-"))
    dest = shelf / (datetime.now().strftime("%Y%m%d-%H%M%S") + "-" + uuid.uuid4().hex[:6])
    try:
        progress("export", 0)
        if "xml" in formats:
            write_xml(temp / "sequence.xml", meta, clips, vertical)
        json_rows = []
        csv_rows = []
        for c in clips:
            cat = normalize_category(c.get("category", "funny"))
            cats = [normalize_category(x) for x in (c.get("categories") or [cat])]
            tags = normalize_hashtags(c.get("hashtags"), cat)
            post = format_post(c, "pt")
            base_row = dict(
                start=c["start"],
                end=c["end"],
                name=c["name"],
                score=c["score"],
                peak=c["peak"],
                category=cat,
                category_label=category_label(cat, "pt"),
                context_note=c.get("context_note", ""),
                description=c.get("description", ""),
            )
            json_rows.append(dict(base_row, categories=cats, hashtags=tags, post=post))
            csv_rows.append(
                dict(
                    base_row,
                    categories=", ".join(category_label(x, "pt") for x in cats),
                    hashtags=" ".join(tags),
                    post=post,
                )
            )
        if "posts" in formats:
            (temp / "posts.txt").write_text(build_posts_text(clips), encoding="utf-8")
        if "json" in formats:
            save(temp / "clips.json", json_rows)
        if "csv" in formats:
            with (temp / "clips.csv").open("w", encoding="utf-8-sig", newline="") as out:
                writer = csv.DictWriter(out, fieldnames=list(csv_rows[0]))
                writer.writeheader()
                writer.writerows(csv_rows)
        if "mp4" in formats:
            for i, c in enumerate(clips, 1):
                progress("render", (i - 1) / len(clips))
                name = re.sub(r'[<>:"/\\|?*\x00-\x1f]', "_", c["name"])[:70].rstrip(". ")
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
                        "pad=ceil(iw/2)*2:ceil(ih/2)*2",
                        "-c:v",
                        "libx264",
                        "-crf",
                        18,
                        "-preset",
                        "veryfast",
                        "-pix_fmt",
                        "yuv420p",
                        "-c:a",
                        "aac",
                        "-aac_coder",
                        "fast",
                        "-b:a",
                        "192k",
                        "-movflags",
                        "+faststart",
                    ],
                    temp / f"{i:03d}-{name}.mp4",
                )
            progress("render", 1)
        progress("export", 1)
        temp.rename(dest)
        return dest
    except BaseException:
        shutil.rmtree(temp, ignore_errors=True)
        raise
