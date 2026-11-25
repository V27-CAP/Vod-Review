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

    def test_posts_csv_and_json_export(self):
        import csv
        import json
        from unittest import mock

        from vodcut import export as export_mod
        from vodcut.core import identity

        with tempfile.TemporaryDirectory() as tmp:
            tmp_path = Path(tmp)
            source = tmp_path / "live.mp4"
            source.write_bytes(b"fake-video-bytes")
            project = dict(
                project=identity(source),
                run="run1",
                meta=dict(
                    path=str(source),
                    duration=120,
                    fps="30",
                    width=1920,
                    height=1080,
                    channels=2,
                    audio_rate=48000,
                    video_index=0,
                ),
                clips=[
                    dict(
                        id="p10",
                        name="Momento hilário",
                        start=10.0,
                        end=35.0,
                        peak=20.5,
                        score=0.87,
                        status="approved",
                        category="funny",
                        categories=["funny", "intense"],
                        context_note="Erro inesperado no chefe final.",
                        description="Não acredito que isso aconteceu ao vivo!",
                        hashtags=["#live", "#engracado", "#shorts"],
                    )
                ],
            )
            with mock.patch.object(export_mod, "CACHE", tmp_path / "cache"):
                dest = export_mod.deliver(project, ["posts", "csv", "json"], False, lambda *_: None)
                posts_txt = (dest / "posts.txt").read_text(encoding="utf-8")
                self.assertIn("Momento hilário", posts_txt)
                self.assertIn("[Engraçado]", posts_txt)
                self.assertIn("Não acredito que isso aconteceu ao vivo!", posts_txt)
                self.assertIn("Contexto: Erro inesperado no chefe final.", posts_txt)
                self.assertIn("#live #engracado #shorts", posts_txt)

                with (dest / "clips.csv").open(encoding="utf-8-sig") as f:
                    rows = list(csv.DictReader(f))
                self.assertEqual(len(rows), 1)
                self.assertEqual(rows[0]["category"], "funny")
                self.assertEqual(rows[0]["category_label"], "Engraçado")
                self.assertIn("#engracado", rows[0]["hashtags"])
                self.assertIn("Erro inesperado", rows[0]["context_note"])

                data = json.loads((dest / "clips.json").read_text(encoding="utf-8"))
                self.assertEqual(data[0]["category"], "funny")
                self.assertEqual(data[0]["categories"], ["funny", "intense"])
                self.assertEqual(data[0]["hashtags"], ["#live", "#engracado", "#shorts"])
                self.assertIn("Momento hilário", data[0]["post"])
