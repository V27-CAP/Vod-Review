import unittest

import numpy as np

from vodcut.core import config
from vodcut.detector import bounds, normalize, rank, snippet


class DetectionTests(unittest.TestCase):
    def setUp(self):
        self.cfg = config()

    def test_constant_and_sparse_signals(self):
        feature = self.cfg["features"]
        self.assertFalse(normalize(np.ones(100), feature).any())
        values = np.zeros(400)
        values[5] = 1
        self.assertEqual(normalize(values, feature).sum(), 1)

    def test_context_and_suppression(self):
        audio = np.zeros(180)
        audio[[20, 26, 120]] = 1
        words = np.zeros(180)
        words[120] = 1
        clips, _ = rank({"loudness": audio, "transcript": words}, [], [], [], 180, self.cfg)
        self.assertEqual(len(clips), 2)
        self.assertEqual(clips[0]["peak"], 120.5)
        self.assertTrue(clips[1]["end"] <= clips[0]["start"])
        self.assertFalse(rank({"loudness": np.ones(40)}, [], [], [], 40, self.cfg)[0])

    def test_natural_boundaries_and_edges(self):
        c = self.cfg["clips"]
        self.assertEqual(bounds(35, 100, [19], [46], c), (19, 46))
        for total in [3, 15, 19, 400]:
            for peak in [0, total / 2, total - 0.001]:
                a, b = bounds(peak, total, [0], [total], c)
                self.assertTrue(0 <= a <= peak <= b <= total)
                self.assertTrue(min(total, c["min_seconds"]) <= b - a <= c["max_seconds"])

    def test_snippet_uses_cut_words(self):
        s = dict(
            start=0,
            end=3,
            text="first last",
            words=[dict(start=0, end=1, text="first"), dict(start=2, end=3, text=" last")],
        )
        self.assertEqual(snippet([s], 1.9, 3), "last")
