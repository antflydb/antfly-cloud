#!/usr/bin/env python3
# /// script
# requires-python = ">=3.11"
# dependencies = []
# ///
"""
Compare Gemini API vs Termite benchmark results.

Usage:
    uv run benchmark/runs/1k-head-to-head/compare.py
    uv run benchmark/runs/1k-head-to-head/compare.py --gemini results/other.jsonl
"""

import argparse
import csv
import json
import statistics
import sys
from pathlib import Path


def load_jsonl(path: Path) -> list[dict]:
    records = []
    with open(path) as f:
        for line in f:
            line = line.strip()
            if line:
                records.append(json.loads(line))
    return records


def as_list(val) -> list[str]:
    """Normalize tags/action to a list of strings."""
    if isinstance(val, list):
        return [str(v).strip().lower() for v in val if v]
    if isinstance(val, str):
        return [x.strip().lower() for x in val.split(",") if x.strip()]
    return []


def jaccard(a: set, b: set) -> float:
    if not a and not b:
        return 1.0
    union = a | b
    return len(a & b) / len(union) if union else 0.0


EXPECTED_FIELDS = [
    "literal", "source", "mood", "action", "context", "tags",
    "characters", "meme_reference", "visual_style", "intensity", "suitable_for",
]


def field_completeness(records: list[dict]) -> dict[str, float]:
    """Percentage of records that have each expected field non-empty."""
    if not records:
        return {f: 0.0 for f in EXPECTED_FIELDS}
    counts = {f: 0 for f in EXPECTED_FIELDS}
    for r in records:
        for f in EXPECTED_FIELDS:
            val = r.get(f)
            if val is not None and val != "" and val != []:
                counts[f] += 1
    return {f: counts[f] / len(records) for f in EXPECTED_FIELDS}


def engine_stats(records: list[dict], label: str) -> dict:
    """Compute aggregate stats for one engine's results."""
    tag_counts = [len(as_list(r.get("tags"))) for r in records]
    literal_lens = [len(r.get("literal", "")) for r in records]
    source_identified = sum(1 for r in records
                           if r.get("source") and r["source"].lower() not in ("unknown", "n/a", ""))

    stats = {
        "engine": label,
        "total_records": len(records),
        "field_completeness": field_completeness(records),
        "tags_mean": round(statistics.mean(tag_counts), 1) if tag_counts else 0,
        "tags_median": statistics.median(tag_counts) if tag_counts else 0,
        "literal_len_mean": round(statistics.mean(literal_lens), 1) if literal_lens else 0,
        "literal_len_median": round(statistics.median(literal_lens), 1) if literal_lens else 0,
        "source_identified_pct": round(source_identified / len(records) * 100, 1) if records else 0,
    }

    # Intensity distribution
    intensities = [r.get("intensity") for r in records if r.get("intensity") is not None]
    if intensities:
        int_vals = [int(v) for v in intensities if str(v).isdigit()]
        stats["intensity_distribution"] = {str(i): int_vals.count(i) for i in range(1, 6)}

    return stats


def main():
    script_dir = Path(__file__).parent
    results_dir = script_dir / "results"

    parser = argparse.ArgumentParser(description="Compare benchmark results")
    parser.add_argument("--gemini", type=Path,
                        default=results_dir / "gemini-1k-5frame.jsonl")
    parser.add_argument("--termite", type=Path,
                        default=results_dir / "termite-1k-5frame.jsonl")
    args = parser.parse_args()

    # Load results
    if not args.gemini.exists():
        print(f"Error: Gemini results not found: {args.gemini}", file=sys.stderr)
        sys.exit(1)
    if not args.termite.exists():
        print(f"Error: Termite results not found: {args.termite}", file=sys.stderr)
        sys.exit(1)

    gemini = load_jsonl(args.gemini)
    termite = load_jsonl(args.termite)

    gemini_by_path = {r["source_path"]: r for r in gemini}
    termite_by_path = {r["source_path"]: r for r in termite}

    both_paths = set(gemini_by_path.keys()) & set(termite_by_path.keys())
    gemini_only = set(gemini_by_path.keys()) - set(termite_by_path.keys())
    termite_only = set(termite_by_path.keys()) - set(gemini_by_path.keys())

    # ---- Per-engine stats ----
    g_stats = engine_stats(gemini, "gemini")
    t_stats = engine_stats(termite, "termite")

    # ---- Cross-engine comparison (matched GIFs only) ----
    mood_matches = 0
    source_matches = 0
    tag_jaccards = []
    detail_rows = []

    for path in sorted(both_paths):
        g = gemini_by_path[path]
        t = termite_by_path[path]

        g_tags = set(as_list(g.get("tags")))
        t_tags = set(as_list(t.get("tags")))
        tj = jaccard(g_tags, t_tags)
        tag_jaccards.append(tj)

        g_mood = (g.get("mood") or "").strip().lower()
        t_mood = (t.get("mood") or "").strip().lower()
        mood_match = g_mood == t_mood
        if mood_match:
            mood_matches += 1

        g_source = (g.get("source") or "").strip().lower()
        t_source = (t.get("source") or "").strip().lower()
        source_match = g_source == t_source and g_source not in ("", "unknown", "n/a")
        if source_match:
            source_matches += 1

        detail_rows.append({
            "source_path": path,
            "dataset": g.get("dataset", ""),
            "gemini_mood": g.get("mood", ""),
            "termite_mood": t.get("mood", ""),
            "gemini_source": g.get("source", ""),
            "termite_source": t.get("source", ""),
            "gemini_tags": "|".join(sorted(g_tags)),
            "termite_tags": "|".join(sorted(t_tags)),
            "gemini_tag_count": len(g_tags),
            "termite_tag_count": len(t_tags),
            "gemini_literal_len": len(g.get("literal", "")),
            "termite_literal_len": len(t.get("literal", "")),
            "tag_jaccard": round(tj, 3),
            "mood_match": mood_match,
            "source_match": source_match,
        })

    n_both = len(both_paths)
    comparison = {
        "matched_gifs": n_both,
        "gemini_only": len(gemini_only),
        "termite_only": len(termite_only),
        "mood_agreement_pct": round(mood_matches / n_both * 100, 1) if n_both else 0,
        "source_agreement_pct": round(source_matches / n_both * 100, 1) if n_both else 0,
        "tag_jaccard_mean": round(statistics.mean(tag_jaccards), 3) if tag_jaccards else 0,
        "tag_jaccard_median": round(statistics.median(tag_jaccards), 3) if tag_jaccards else 0,
    }

    # ---- Write summary JSON ----
    summary = {
        "gemini": g_stats,
        "termite": t_stats,
        "comparison": comparison,
    }
    summary_path = results_dir / "comparison-summary.json"
    with open(summary_path, "w") as f:
        json.dump(summary, f, indent=2)

    # ---- Write detail CSV ----
    detail_path = results_dir / "comparison-detail.csv"
    if detail_rows:
        with open(detail_path, "w", newline="") as f:
            w = csv.DictWriter(f, fieldnames=detail_rows[0].keys())
            w.writeheader()
            w.writerows(detail_rows)

    # ---- Console output ----
    print("=" * 60)
    print("1K GIF Benchmark: Gemini API vs Termite")
    print("=" * 60)

    print(f"\n  Gemini records:  {g_stats['total_records']}")
    print(f"  Termite records: {t_stats['total_records']}")
    print(f"  Matched (both):  {n_both}")
    if gemini_only:
        print(f"  Gemini only:     {len(gemini_only)}")
    if termite_only:
        print(f"  Termite only:    {len(termite_only)}")

    print(f"\n--- Quality ---")
    print(f"  Mood agreement:    {comparison['mood_agreement_pct']}%")
    print(f"  Source agreement:  {comparison['source_agreement_pct']}%")
    print(f"  Tag Jaccard mean:  {comparison['tag_jaccard_mean']}")
    print(f"  Tag Jaccard median:{comparison['tag_jaccard_median']}")

    print(f"\n--- Tags ---")
    print(f"  Gemini mean/median:  {g_stats['tags_mean']} / {g_stats['tags_median']}")
    print(f"  Termite mean/median: {t_stats['tags_mean']} / {t_stats['tags_median']}")

    print(f"\n--- Literal length ---")
    print(f"  Gemini mean/median:  {g_stats['literal_len_mean']} / {g_stats['literal_len_median']}")
    print(f"  Termite mean/median: {t_stats['literal_len_mean']} / {t_stats['literal_len_median']}")

    print(f"\n--- Source identification ---")
    print(f"  Gemini:  {g_stats['source_identified_pct']}%")
    print(f"  Termite: {t_stats['source_identified_pct']}%")

    print(f"\n--- Field completeness ---")
    for field in EXPECTED_FIELDS:
        g_pct = round(g_stats["field_completeness"].get(field, 0) * 100)
        t_pct = round(t_stats["field_completeness"].get(field, 0) * 100)
        print(f"  {field:20s}  gemini: {g_pct:3d}%  termite: {t_pct:3d}%")

    print(f"\nWrote: {summary_path}")
    print(f"Wrote: {detail_path}")


if __name__ == "__main__":
    main()
