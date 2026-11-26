import csv
import http.server
import json
import tempfile
import threading
import unittest
from pathlib import Path
from unittest import mock

import numpy as np

from vodcut import export as export_mod
from vodcut.classify import classify_clip
from vodcut.core import apply_preset, config, identity
from vodcut.detector import rank
from vodcut.judge import AIJudge
from vodcut.signals import speech_features


class PresetsAndEnglishTests(unittest.TestCase):
    def test_apply_presets_and_validation(self):
        cfg = config()
        apply_preset(cfg, "funny_moments")
        self.assertEqual(cfg["analysis"]["preset"], "funny_moments")
        self.assertEqual(cfg["clips"]["min_seconds"], 5)
        self.assertEqual(cfg["clips"]["max_seconds"], 55)
        self.assertEqual(cfg["clips"]["top_n"], 60)
        self.assertEqual(cfg["clips"]["merge_gap"], 8)
        self.assertEqual(cfg["judge"]["style"], "funny_moments")

        apply_preset(cfg, "best_moments")
        self.assertEqual(cfg["analysis"]["preset"], "best_moments")
        self.assertEqual(cfg["clips"]["min_seconds"], 10)
        self.assertEqual(cfg["clips"]["max_seconds"], 240)
        self.assertEqual(cfg["clips"]["merge_gap"], 20)
        self.assertLess(cfg["judge"]["understandability_weight"], 1.0)

        with self.assertRaises(ValueError):
            apply_preset(cfg, "nonexistent_preset")

    def test_best_moments_merges_nearby_peaks_into_continuous_scene(self):
        cfg = config()
        apply_preset(cfg, "best_moments")
        # Three peaks within 35 seconds should merge into one continuous scene under best_moments (merge_gap=20)
        audio = np.zeros(300)
        audio[[50, 72, 95, 220]] = 1.0
        clips, _ = rank({"loudness": audio}, [], [], [], 300, cfg)
        self.assertEqual(len(clips), 2)
        merged_scene = next(c for c in clips if c["start"] < 60)
        self.assertGreaterEqual(merged_scene.get("merged_peaks", 1), 2)
        self.assertGreater(merged_scene["end"], 95)
        self.assertLessEqual(merged_scene["end"] - merged_scene["start"], cfg["clips"]["max_seconds"])
        # Strict non-overlap
        ordered = sorted(clips, key=lambda c: c["start"])
        self.assertLessEqual(ordered[0]["end"], ordered[1]["start"])

    def test_single_highlight_finishes_scene(self):
        cfg = config()
        apply_preset(cfg, "best_moments")
        audio = np.zeros(300)
        audio[[50, 72, 95, 220]] = 1
        cfg["clips"]["top_n"] = 1
        clips, _ = rank({"loudness": audio}, [], [], [], 300, cfg)
        self.assertEqual(len(clips), 1)
        self.assertGreater(clips[0]["end"], 95)

    def test_neighbor_that_exceeds_merge_limit_remains_a_candidate(self):
        cfg = config()
        apply_preset(cfg, "funny_moments")
        audio = np.zeros(300)
        audio[[50, 64, 78, 92, 106]] = 1
        clips, _ = rank({"loudness": audio}, [], [], [], 300, cfg)
        self.assertEqual(len(clips), 2)
        self.assertTrue(any(c["start"] <= 106 < c["end"] for c in clips))

    def test_disconnected_ai_preserves_local_results(self):
        from vodcut import detector
        cfg = config()
        cfg["analysis"].update(judge=True, visual=False, facecam=False)
        cfg["judge"].update(provider="lmstudio", endpoint="http://127.0.0.1:1234/v1/chat/completions")
        segments = [{"start": 15., "end": 18., "text": "OH MY GOD [screaming]"}]
        with tempfile.TemporaryDirectory() as tmp:
            video = Path(tmp) / "vod.mkv"
            video.touch()
            with (
                mock.patch.object(detector, "CACHE", Path(tmp)),
                mock.patch.object(detector, "probe", return_value={"duration": 60, "video_index": 0}),
                mock.patch.object(detector, "prepare", return_value=([], segments)),
                mock.patch.object(detector, "ffmpeg"),
                mock.patch.object(AIJudge, "rate", side_effect=ConnectionRefusedError()),
            ):
                result = detector.detect(video, cfg)
                self.assertTrue(result["meta"]["ai_unavailable"])
                cfg["analysis"]["judge"] = False
                self.assertEqual(result["clips"], detector.detect(video, cfg)["clips"])

    def test_english_reaction_pattern_precision_and_span(self):
        cfg = config()
        segments = [
            {"start": 5.0, "end": 9.0, "text": "Wait, what is the altitude right now on the radar"},
            {"start": 20.0, "end": 24.0, "text": "OH MY GOD [screaming] where is he?!"},
            {"start": 40.0, "end": 44.0, "text": "I'm not crying, you're crying [laughter]"},
        ]
        scores = speech_features(segments, 60, cfg["features"])
        # Routine chatter with bare "wait" / "what" should not trigger false positives
        self.assertEqual(scores[7], 0.0)
        # Reaction segments should fire across their duration (not only at segment end)
        self.assertGreater(scores[21], 0.8)
        self.assertGreater(scores[23], 0.8)
        self.assertGreater(scores[41], 0.8)

    def test_english_offline_classification_and_export(self):
        emotional = classify_clip(
            dict(
                start=100,
                end=150,
                context=True,
                transcript="I'm not crying, you're crying. God, this game is so sincere. Thank you everyone for watching the finale.",
                breakdown={"loudness": 0.2, "transcript": 0.8},
                signals=["transcript"],
            ),
            lang="en",
        )
        self.assertEqual(emotional["category"], "story")
        self.assertIn("Memorable story moment", emotional["context_note"])
        self.assertIn("#vtuber", emotional["hashtags"])

        panic = classify_clip(
            dict(
                start=10,
                end=20,
                transcript="OH MY GOD! [screaming] MISSILE WARNING! NO NO NO!",
                breakdown={"loudness": 0.9, "vocal": 0.9, "change": 0.8},
                signals=["loudness", "vocal"],
            ),
            lang="en",
        )
        self.assertEqual(panic["category"], "intense")
        self.assertIn("Intense moment", panic["context_note"])

        with tempfile.TemporaryDirectory() as tmp:
            tmp_path = Path(tmp)
            source = tmp_path / "vtuber-vod.mkv"
            source.write_bytes(b"fake-vod")
            project = dict(
                project=identity(source),
                run="run-en",
                meta=dict(
                    path=str(source),
                    duration=200,
                    fps="60",
                    width=1920,
                    height=1080,
                    channels=2,
                    audio_rate=48000,
                    video_index=0,
                    output_language="en",
                    preset="funny_moments",
                ),
                clips=[
                    dict(
                        id="p15",
                        name="Missile panic fail",
                        start=10.0,
                        end=20.0,
                        peak=15.5,
                        score=0.91,
                        status="approved",
                        category="intense",
                        categories=["intense", "funny"],
                        context_note=panic["context_note"],
                        description=panic["description"],
                        hashtags=panic["hashtags"],
                    )
                ],
            )
            with mock.patch.object(export_mod, "CACHE", tmp_path / "cache"):
                dest = export_mod.deliver(project, ["posts", "csv", "json"], False, lambda *_: None)
                posts = (dest / "posts.txt").read_text(encoding="utf-8")
                self.assertIn("[Intense]", posts)
                self.assertIn("Context: ", posts)
                with (dest / "clips.csv").open(encoding="utf-8-sig") as f:
                    rows = list(csv.DictReader(f))
                self.assertEqual(rows[0]["category_label"], "Intense")

    def test_judge_prompt_adapts_to_style_and_english_output(self):
        captured = []

        class Handler(http.server.BaseHTTPRequestHandler):
            def do_POST(self):
                body = json.loads(self.rfile.read(int(self.headers.get("Content-Length", "0"))))
                captured.append(body)
                ai_json = json.dumps(
                    {
                        "funny": 0.92,
                        "surprising": 0.8,
                        "understandable": 0.75,
                        "reason_pt": "Falha hilária no pouso.",
                        "reason_en": "Hilarious crash landing fail.",
                        "category": "funny",
                        "categories": ["funny", "intense"],
                        "title": "I'm a Goddamn Fraud",
                        "context_note": "Streamer crashes upside down while bragging about flying skills.",
                        "description": "Absolute ace pilot energy right here.",
                        "hashtags": ["#vtuber", "#funny", "#clips", "#gaming"],
                    }
                )
                data = json.dumps({"choices": [{"message": {"content": ai_json}}]}).encode()
                self.send_response(200)
                self.send_header("Content-Type", "application/json")
                self.send_header("Content-Length", str(len(data)))
                self.end_headers()
                self.wfile.write(data)

            def log_message(self, *args):
                pass

        server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), Handler)
        port = server.server_address[1]
        threading.Thread(target=server.serve_forever, daemon=True).start()
        try:
            cfg = config()
            apply_preset(cfg, "funny_moments")
            jcfg = cfg["judge"] | {
                "provider": "lmstudio",
                "endpoint": f"http://127.0.0.1:{port}/v1/chat/completions",
                "output_language": "en",
            }
            out = AIJudge(jcfg).rate("OH MY GOD I'm a goddamn fraud!")
            self.assertEqual(out["title"], "I'm a Goddamn Fraud")
            system_prompt = captured[-1]["messages"][0]["content"]
            self.assertIn("Funny Moments", system_prompt)
            self.assertIn("in English", system_prompt)
        finally:
            server.shutdown()
            server.server_close()
