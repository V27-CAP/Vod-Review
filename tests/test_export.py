import tempfile
import unittest
import xml.etree.ElementTree as ET
from pathlib import Path

from vodcut.export import validate_xml, write_xml


class ExportTests(unittest.TestCase):
    def test_ntsc_stereo_links_markers_and_name(self):
        with tempfile.TemporaryDirectory() as tmp:
            path = Path(tmp) / "s.xml"
            meta = dict(
                path=str(Path(tmp) / "original ê.mov"),
                duration=120,
                fps="30000/1001",
                width=1920,
                height=1080,
                channels=2,
                audio_rate=48000,
                timecode="01:00:00;00",
            )
            clips = [
                dict(name="A & B", start=5, end=12, peak=7),
                dict(name="C", start=50, end=55, peak=54.9999),
            ]
            write_xml(path, meta, clips, True)
            root = ET.parse(path)
            self.assertEqual(root.findtext(".//sequence/rate/ntsc"), "TRUE")
            self.assertEqual(root.findtext(".//video/format/samplecharacteristics/width"), "1080")
            self.assertEqual(root.findtext(".//file/media/video/samplecharacteristics/width"), "1920")
            self.assertEqual(root.findtext(".//file/timecode/frame"), "107892")
            self.assertEqual(len(root.findall(".//clipitem")), 6)
            self.assertEqual(root.findtext(".//clipitem/name"), "A & B")
            self.assertEqual(
                root.findtext(".//marker/in"), str(round(7 * 30000 / 1001) - round(5 * 30000 / 1001))
            )
            root.find(".//marker/in").text = "-1"
            root.write(path)
            with self.assertRaises(ValueError):
                validate_xml(path)

    def test_silent_source_uses_native_size(self):
        with tempfile.TemporaryDirectory() as tmp:
            path = Path(tmp) / "s.xml"
            write_xml(
                path,
                dict(path="silent.mkv", duration=20, fps="25", width=1280, height=720, channels=0),
                [dict(name="X", start=2, end=3, peak=2.5)],
                False,
            )
            root = ET.parse(path)
            self.assertIsNone(root.find(".//audio"))
            self.assertEqual(root.findtext(".//video/format/samplecharacteristics/height"), "720")
            self.assertEqual(root.findtext(".//sequence/rate/ntsc"), "FALSE")
