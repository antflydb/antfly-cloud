#!/usr/bin/env python3
# /// script
# requires-python = ">=3.11"
# dependencies = ["httpx", "python-dotenv"]
# ///
"""
Apply mood_emoji classifications to Antfly documents using the transforms API.

Uses $set transforms to update ONLY the mood_emoji field on each document,
without touching any other fields. This is a partial update — no data loss.

Usage:
    uv run ingest/classify-moods/apply.py --jsonl ingest/image-to-text/output/descriptions-gemini-2.5-flash-lite.jsonl
    uv run ingest/classify-moods/apply.py --jsonl ... --dry-run
"""

import argparse
import json
import os
import sys
import time
from pathlib import Path

from dotenv import load_dotenv
import httpx

load_dotenv()

ANTFLY_URL = os.environ.get("ANTFLY_URL", "http://localhost:8080/api/v1")
TABLE_NAME = os.environ.get("INGEST_TABLE", "honeycomb")
BATCH_SIZE = 200


def flush_batch(client: httpx.Client, table: str, transforms: list) -> None:
    """Apply $set transforms to update mood_emoji on documents."""
    resp = client.post(
        f"/tables/{table}/batch",
        json={"transforms": transforms},
        timeout=60.0,
    )
    resp.raise_for_status()


def main():
    parser = argparse.ArgumentParser(description="Apply mood_emoji field to Antfly documents")
    parser.add_argument("--jsonl", required=True, help="Path to descriptions JSONL file")
    parser.add_argument("--mapping", type=Path, default=Path(__file__).parent / "output/mood_mapping.json",
                        help="Path to mood_mapping.json")
    parser.add_argument("--table", default=TABLE_NAME, help=f"Antfly table name (default: {TABLE_NAME})")
    parser.add_argument("--url", default=ANTFLY_URL, help=f"Antfly API URL")
    parser.add_argument("--limit", type=int, default=0, help="Limit docs to process (0=all)")
    parser.add_argument("--batch-size", type=int, default=BATCH_SIZE, help=f"Batch size (default: {BATCH_SIZE})")
    parser.add_argument("--dry-run", action="store_true", help="Show what would be done without writing")
    args = parser.parse_args()

    # Load mood mapping
    if not args.mapping.exists():
        print(f"Error: mood mapping not found: {args.mapping}", file=sys.stderr)
        print("Run classify.py first to generate mood_mapping.json", file=sys.stderr)
        sys.exit(1)

    with open(args.mapping) as f:
        mood_mapping = json.load(f)
    print(f"Loaded {len(mood_mapping):,} mood classifications")

    # Keyword fallback (import from classify.py categories)
    from classify import keyword_fallback

    if args.dry_run:
        # Just show stats
        total = 0
        matched = 0
        fallback = 0
        no_mood = 0
        with open(args.jsonl) as f:
            for line in f:
                total += 1
                doc = json.loads(line)
                mood = doc.get("mood", "").strip()
                if mood in mood_mapping:
                    matched += 1
                elif mood:
                    fallback += 1
                else:
                    no_mood += 1
        print(f"  {total:,} docs: {matched:,} mapped, {fallback:,} would use fallback, {no_mood:,} no mood")
        return

    client = httpx.Client(base_url=args.url, timeout=30.0)

    batch: list = []
    updated = 0
    start = time.time()
    fallback_count = 0

    print(f"Patching mood_emoji field via transforms API from {args.jsonl}...")

    with open(args.jsonl) as f:
        for line in f:
            desc = json.loads(line)
            doc_id = desc.get("id", "")
            if not doc_id:
                continue

            # Determine mood_emoji category
            mood = desc.get("mood", "").strip()
            if mood in mood_mapping:
                category = mood_mapping[mood]
            elif mood:
                category = keyword_fallback(mood)
                fallback_count += 1
            else:
                category = "playful"  # default for missing mood

            # Use $set transform to update only the mood_emoji field
            batch.append({
                "key": doc_id,
                "operations": [
                    {"op": "$set", "path": "$.mood_emoji", "value": category}
                ]
            })

            if len(batch) >= args.batch_size:
                try:
                    flush_batch(client, args.table, batch)
                    updated += len(batch)
                except httpx.HTTPError as e:
                    print(f"\nWarning: batch failed ({len(batch)} docs): {e}", file=sys.stderr)
                batch = []

                elapsed = time.time() - start
                rate = updated / elapsed if elapsed > 0 else 0
                print(f"\rPatched: {updated:,} ({rate:.1f}/sec)", end="", flush=True)

                if args.limit and updated >= args.limit:
                    print(f"\nReached limit of {args.limit}")
                    break

    # Final batch
    if batch:
        try:
            flush_batch(client, args.table, batch)
            updated += len(batch)
        except httpx.HTTPError as e:
            print(f"\nWarning: final batch failed ({len(batch)} docs): {e}", file=sys.stderr)

    elapsed = time.time() - start
    rate = updated / elapsed if elapsed > 0 else 0
    print(f"\nCompleted: {updated:,} docs patched in {elapsed:.1f}s ({rate:.1f}/sec)")
    if fallback_count:
        print(f"  {fallback_count:,} used keyword fallback")


if __name__ == "__main__":
    main()
