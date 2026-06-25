#!/usr/bin/env python3
# /// script
# requires-python = ">=3.10"
# ///
"""Generate a review markdown file from descriptions JSONL.

Supports multiple input files for side-by-side model comparison.
"""

import argparse
import json
from collections import OrderedDict
from pathlib import Path

MEDIA_BASE = "https://media.honeycomb.antfly.io"

# Fields that are metadata, not description content
SKIP_FIELDS = {"id", "dataset", "source_path", "attribution"}


def format_value(v):
    if isinstance(v, list):
        return ", ".join(str(x) for x in v)
    if v is None:
        return "_none_"
    return str(v)


def load_jsonl(path: str) -> list[dict]:
    entries = []
    with open(path) as f:
        for line in f:
            line = line.strip()
            if line:
                entries.append(json.loads(line))
    return entries


def model_name_from_path(path: str) -> str:
    """Extract model name from filename like descriptions-gemini-2.5-flash.jsonl."""
    stem = Path(path).stem  # descriptions-gemini-2.5-flash
    if stem.startswith("descriptions-"):
        return stem[len("descriptions-"):]
    return stem


def entry_fields_markdown(entry: dict) -> str:
    lines = []
    for field, value in entry.items():
        if field not in SKIP_FIELDS:
            lines.append(f"- **{field}**: {format_value(value)}")
    return "\n".join(lines)


def main():
    parser = argparse.ArgumentParser(description="Generate review markdown from descriptions JSONL")
    parser.add_argument("--input", nargs="+", required=True,
                        help="One or more JSONL files (model name inferred from filename)")
    parser.add_argument("--output", default="ingest/image-to-text/output/review.md")
    parser.add_argument("--media-base", default=MEDIA_BASE)
    args = parser.parse_args()

    # Load all inputs, keyed by model name
    models = OrderedDict()
    summaries = {}
    for path in args.input:
        model = model_name_from_path(path)
        models[model] = {e["id"]: e for e in load_jsonl(path)}
        # Load summary if it exists
        summary_path = Path(path).with_suffix(".summary.json")
        if summary_path.exists():
            with open(summary_path) as f:
                summaries[model] = json.load(f)

    model_names = list(models.keys())

    # Collect all GIF IDs in order of first appearance
    seen = OrderedDict()
    for entries_by_id in models.values():
        for gid in entries_by_id:
            if gid not in seen:
                seen[gid] = entries_by_id[gid]
    gif_ids = list(seen.keys())

    # Header
    lines = ["# Honeycomb reviewer", ""]
    lines.append(f"- **GIFs**: {len(gif_ids)}")
    lines.append("")
    if summaries:
        lines.append("| Model | Items | Tokens (in/out) | Cost | Time |")
        lines.append("|-------|------:|----------------:|-----:|-----:|")
        for m in model_names:
            s = summaries.get(m)
            if s:
                tok = f"{s['input_tokens']:,} / {s['output_tokens']:,}"
                cost = f"${s['cost']:.4f}" if s['cost'] > 0 else "unknown"
                time_s = f"{s['elapsed_s']:.1f}s"
                batch = " (batch)" if s.get("batch") else ""
                lines.append(f"| `{s.get('model', m)}`{batch} | {s['items']} | {tok} | {cost} | {time_s} |")
            else:
                lines.append(f"| `{m}` | {len(models[m])} | — | — | — |")
        lines.append("")
    else:
        for m in model_names:
            lines.append(f"- **Model**: `{m}`")
        lines.append("")

    multi = len(model_names) > 1

    for gid in gif_ids:
        # Use first available entry for metadata
        entry = seen[gid]
        gif_url = f"{args.media_base}/{entry['source_path']}"
        lines.extend([f"### {gid}", "", f"![{gid}]({gif_url})", ""])

        if multi:
            for m in model_names:
                e = models[m].get(gid)
                if e:
                    lines.extend([f"#### `{m}`", "", entry_fields_markdown(e), ""])
                else:
                    lines.extend([f"#### `{m}`", "", "_not processed by this model_", ""])
        else:
            lines.extend([entry_fields_markdown(entry), ""])

        lines.extend(["---", ""])

    Path(args.output).write_text("\n".join(lines))
    print(f"Wrote {len(gif_ids)} GIFs ({len(model_names)} model(s)) to {args.output}")


if __name__ == "__main__":
    main()
