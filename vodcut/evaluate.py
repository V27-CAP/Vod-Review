"""Evaluate ranked candidates against editor-selected points or intervals."""

from __future__ import annotations

import argparse
import csv
import json
import logging
from pathlib import Path

from scipy.sparse import csr_matrix
from scipy.sparse.csgraph import maximum_bipartite_matching

from .chat import seconds
from .core import ROOT, config
from .detector import detect


def truth(path: Path, duration: float) -> list[tuple[float, float]]:
    with path.open(encoding="utf-8-sig", newline="") as stream:
        rows = json.load(stream) if path.suffix.lower() == ".json" else list(csv.DictReader(stream))
    if not isinstance(rows, list) or not rows:
        raise ValueError("Provide a nonempty list of manual timestamps or ranges")
    result = []
    for entry in rows:
        if isinstance(entry, dict):
            a = seconds(entry.get("start", entry.get("time")))
            b = seconds(entry["end"]) if entry.get("end") not in (None, "") else a
        else:
            a = b = seconds(entry)
        if not 0 <= a <= b <= duration or a >= duration:
            raise ValueError(f"Ground truth outside this VOD: {entry}")
        result.append((a, b))
    return result


def metrics(clips: list, manual: list, k: int) -> dict:
    if k <= 0 or not manual:
        raise ValueError("K and number of references must be positive")
    proposals = clips[:k]
    edges = []
    for c in proposals:
        edges.append(
            [
                int(c["start"] <= a < c["end"] if a == b else max(c["start"], a) < min(c["end"], b))
                for a, b in manual
            ]
        )
    hits = 0
    if edges:
        matches = maximum_bipartite_matching(csr_matrix(edges), perm_type="column")
        hits = int(sum(matches >= 0))
    return dict(
        k=k,
        returned=len(proposals),
        manual=len(manual),
        hits=hits,
        precision_at_k=hits / k,
        recall_at_k=hits / len(manual),
    )


def main() -> None:
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("vod", type=Path)
    p.add_argument("ground_truth", type=Path)
    p.add_argument("--config", type=Path, default=ROOT / "config.toml")
    p.add_argument("--chat", type=Path)
    p.add_argument("--k", type=int, nargs="+", default=[5, 10, 20])
    p.add_argument("--no-transcription", action="store_true")
    a = p.parse_args()
    if min(a.k) < 1:
        p.error("K must be positive")
    logging.basicConfig(level=logging.INFO)
    cfg = config(a.config)
    cfg["clips"]["top_n"] = max(cfg["clips"]["top_n"], max(a.k))
    if a.no_transcription:
        cfg["analysis"]["transcription"] = False
    proj = detect(a.vod, cfg, a.chat)
    manual = truth(a.ground_truth, proj["meta"]["duration"])
    print(json.dumps([metrics(proj["clips"], manual, k) for k in a.k], indent=2))


if __name__ == "__main__":
    main()
