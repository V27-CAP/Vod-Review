import importlib.util
import json
import subprocess
import tempfile
import unittest
from pathlib import Path

from vodcut.core import config
from vodcut.media import (
    extract_facecam_crop,
    extract_frame,
    normalize_facecam,
    prepare,
    probe,
    select_audio_track,
    voice_filter_chain,
)
from vodcut.signals import facecam_features


def _make_multitrack_video(path: Path) -> None:
    """Create a 10s MKV with 2 audio tracks (mix + OBS Mic) and animated bottom-right facecam box."""
    # Draw a bright rectangle in the bottom-right corner that flashes/moves between t=5 and t=7.
    vf = (
        "drawbox=x=220:y=110:w=90:h=64:color=white@0.9:t=fill:enable='between(t,5,7)',"
        "drawbox=x=240:y=130:w=40:h=16:color=black@1:t=fill:enable='between(t,5.5,6.8)'"
    )
    subprocess.run(
        [
            "ffmpeg",
            "-nostdin",
            "-v",
            "error",
            "-y",
            "-f",
            "lavfi",
            "-i",
            "color=c=0x202030:s=320x180:r=10:d=10",
            "-f",
            "lavfi",
            "-i",
            "sine=frequency=440:duration=10,aformat=channel_layouts=stereo",
            "-f",
            "lavfi",
            "-i",
            "sine=frequency=880:duration=10,aformat=channel_layouts=mono",
            "-map",
            "0:v:0",
            "-map",
            "1:a:0",
            "-map",
            "2:a:0",
            "-vf",
            vf,
            "-metadata:s:a:0",
            "title=Stream Mix",
            "-metadata:s:a:1",
            "title=Mic/Aux",
            "-c:v",
            "libx264",
            "-preset",
            "ultrafast",
            "-pix_fmt",
            "yuv420p",
            "-c:a",
            "aac",
            str(path),
        ],
        check=True,
    )


class VoiceAndFacecamTests(unittest.TestCase):
    def test_voice_filter_chain_modes(self):
        cfg = config()["features"]
        mixed = voice_filter_chain(2, isolate=True, features_cfg=cfg, dedicated_mic=False)
        self.assertIn("stereotools=mlev=1:slev=0.150", mixed)
        self.assertIn("highpass=f=120", mixed)
        self.assertIn("lowpass=f=3800", mixed)
        self.assertIn("afftdn=nf=-25", mixed)
        self.assertTrue(mixed.endswith("aresample=async=1:first_pts=0"))

        mic = voice_filter_chain(2, isolate=True, features_cfg=cfg, dedicated_mic=True)
        self.assertNotIn("stereotools", mic)
        self.assertIn("highpass=f=120", mic)
        self.assertIn("afftdn=nf=-25", mic)

        raw = voice_filter_chain(2, isolate=False, features_cfg=cfg)
        self.assertEqual(raw, "aresample=async=1:first_pts=0")

    def test_normalize_facecam(self):
        self.assertIsNone(normalize_facecam(None))
        self.assertIsNone(normalize_facecam({"x": 0.1, "y": 0.1, "w": 0.005, "h": 0.2}))
        box = normalize_facecam({"x": 0.7, "y": 0.6, "w": 0.5, "h": 0.5})
        self.assertEqual(box, {"x": 0.7, "y": 0.6, "w": 0.3, "h": 0.4})

    def test_multitrack_probe_prepare_facecam_and_worker(self):
        with tempfile.TemporaryDirectory() as tmp:
            tmp_path = Path(tmp)
            video = tmp_path / "obs-vod.mkv"
            _make_multitrack_video(video)

            meta = probe(video)
            self.assertEqual(len(meta["audio_tracks"]), 2)
            self.assertEqual(meta["audio_tracks"][0]["title"], "Stream Mix")
            self.assertEqual(meta["audio_tracks"][1]["title"], "Mic/Aux")
            self.assertEqual(meta["audio_tracks"][1]["channels"], 1)

            default_map, default_ch, default_ded = select_audio_track(meta, -1)
            self.assertEqual((default_map, default_ch, default_ded), ("0:a:0", 2, False))
            mic_map, mic_ch, mic_ded = select_audio_track(meta, 1)
            self.assertEqual((mic_map, mic_ch, mic_ded), ("0:a:1", 1, True))

            cfg = config()
            cfg["analysis"]["transcription"] = False
            cfg["analysis"]["audio_track"] = 1
            cfg["analysis"]["voice_isolation"] = True
            waves, segments = prepare(meta, tmp_path / "cache", cfg, lambda *_: None)
            self.assertEqual(len(waves), 1)
            self.assertTrue(waves[0][1].is_file())
            self.assertEqual(segments, [])

            # Frame extraction and facecam crop
            frame = extract_frame(meta, 6.0, tmp_path / "frame.jpg")
            self.assertTrue(frame.is_file() and frame.stat().st_size > 500)
            roi = {"x": 0.68, "y": 0.60, "w": 0.30, "h": 0.38}
            crop = extract_facecam_crop(meta, 6.0, roi, tmp_path / "face.jpg")
            self.assertTrue(crop.is_file() and crop.stat().st_size > 200)

            # Facecam expression / facial movement peak around t=5..7
            import math

            scores = facecam_features(meta, roi, 1, cfg["features"], lambda *_: None)
            self.assertEqual(len(scores), math.ceil(meta["duration"]))
            peak_sec = max(range(len(scores)), key=lambda i: scores[i])
            self.assertIn(peak_sec, (5, 6, 7, 8))

            # Verify launcher/worker.py probe action
            spec = importlib.util.spec_from_file_location(
                "vodcut_worker", Path(__file__).resolve().parents[1] / "launcher" / "worker.py"
            )
            worker = importlib.util.module_from_spec(spec)
            spec.loader.exec_module(worker)
            probe_out = json.loads(
                worker.run(
                    {
                        "action": "probe",
                        "path": str(video),
                        "timestamp": 6.0,
                        "cancel": str(tmp_path / "cancel"),
                    }
                )
            )
            self.assertEqual(len(probe_out["meta"]["audio_tracks"]), 2)
            self.assertTrue(Path(probe_out["frame"]).is_file())
