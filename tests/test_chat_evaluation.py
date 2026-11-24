import json
import tempfile
import unittest
from pathlib import Path

from vodcut.chat import chat_features
from vodcut.core import config
from vodcut.evaluate import metrics
from vodcut.judge import LocalJudge, rerank


class AnalysisTests(unittest.TestCase):
    def test_chat_formats_and_alignment(self):
        with tempfile.TemporaryDirectory() as d:
            user = "one"
            docs = [
                ("regular.json", [{"time_in_seconds": 18, "author": {"name": user}, "message": "KEKW"}]),
                (
                    "twitch.json",
                    {
                        "comments": [
                            {
                                "content_offset_seconds": 18,
                                "commenter": {"id": user},
                                "message": {"body": "kkkk"},
                            }
                        ]
                    },
                ),
            ]
            paths = []
            for filename, data in docs:
                path = Path(d) / filename
                path.write_text(json.dumps(data))
                paths.append(path)
            csvpath = Path(d) / "input.csv"
            csvpath.write_text("time,user,message\n00:00:18,one,ABSOLUTELY\n")
            for path in paths + [csvpath]:
                s = chat_features(path, 90, config()["features"])
                for values in s.values():
                    self.assertTrue(values[15] > 0)
                    self.assertEqual(values[10], 0)

    def test_points_intervals_and_one_to_one_assignment(self):
        clips = [{"start": 1, "end": 4}, {"start": 3, "end": 5}]
        self.assertEqual(
            metrics(clips, [(3.5, 3.5), (2, 2)], 4),
            {"k": 4, "returned": 2, "manual": 2, "hits": 2, "precision_at_k": 0.5, "recall_at_k": 1},
        )
        self.assertEqual(metrics(clips, [(4, 7)], 1)["hits"], 0)
        self.assertEqual(metrics(clips, [(3.5, 3.5)], 2)["hits"], 1)
        self.assertEqual(metrics([], [(1, 1)], 5)["recall_at_k"], 0)

    def test_local_judge_and_cache(self):
        class Fake:
            calls = 0

            def rate(self, text):
                self.calls += 1
                return dict(
                    funny=0.8, surprising=0.5, understandable=1, reason_pt="Reação", reason_en="Reaction"
                )

        cfg = config()["judge"]
        with self.assertRaises(ValueError):
            LocalJudge(cfg | {"endpoint": "https://example.com/v1/chat/completions"})
        judge = Fake()
        with tempfile.TemporaryDirectory() as tmp:
            for _ in range(2):
                clip = dict(score=0.3, transcript="Wait!", breakdown={})
                ranked = rerank([clip], cfg, Path(tmp), lambda *_: None, judge)
                self.assertGreater(ranked[0]["score"], 0.3)
            self.assertEqual(judge.calls, 1)
