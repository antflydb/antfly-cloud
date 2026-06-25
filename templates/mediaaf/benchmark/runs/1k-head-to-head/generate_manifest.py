#!/usr/bin/env python3
# /// script
# requires-python = ">=3.11"
# dependencies = ["boto3"]
# ///
"""
Generate a locked manifest of GIF paths from R2 for reproducible benchmark runs.

Usage:
    uv run benchmark/runs/1k-head-to-head/generate_manifest.py
    uv run benchmark/runs/1k-head-to-head/generate_manifest.py --count 500
    uv run benchmark/runs/1k-head-to-head/generate_manifest.py --force  # regenerate
"""

import argparse
import hashlib
import json
import os
import sys
from pathlib import Path

MEDIA_EXTENSIONS = (".gif", ".mp4", ".webm")


def main():
    parser = argparse.ArgumentParser(description="Generate GIF manifest from R2")
    parser.add_argument("--count", type=int, default=1000,
                        help="Number of GIFs to include (default: 1000)")
    parser.add_argument("--bucket", default="honeycomb-media",
                        help="R2 bucket name")
    parser.add_argument("--prefix", default="sources/",
                        help="R2 key prefix")
    parser.add_argument("--force", action="store_true",
                        help="Overwrite existing manifest")
    args = parser.parse_args()

    script_dir = Path(__file__).parent
    manifest_path = script_dir / "manifest.json"

    # Check for existing manifest
    if manifest_path.exists() and not args.force:
        existing_hash = hashlib.sha256(manifest_path.read_bytes()).hexdigest()[:16]
        existing = json.loads(manifest_path.read_text())
        print(f"Manifest already exists: {manifest_path}")
        print(f"  {len(existing)} items, sha256: {existing_hash}...")
        print(f"  Use --force to regenerate")
        sys.exit(0)

    # List objects from R2
    import boto3

    s3 = boto3.client(
        "s3",
        endpoint_url=os.environ.get("R2_ENDPOINT_URL"),
        aws_access_key_id=os.environ.get("R2_ACCESS_KEY_ID"),
        aws_secret_access_key=os.environ.get("R2_SECRET_ACCESS_KEY"),
    )

    print(f"Listing {args.bucket}/{args.prefix} ...")
    paginator = s3.get_paginator("list_objects_v2")
    items = []
    datasets = {}

    for page in paginator.paginate(Bucket=args.bucket, Prefix=args.prefix):
        for obj in page.get("Contents", []):
            key = obj["Key"]
            if not key.lower().endswith(MEDIA_EXTENSIONS):
                continue

            # Extract dataset from path: sources/<dataset>/... -> <dataset>
            parts = key.removeprefix(args.prefix).split("/")
            dataset = parts[0] if len(parts) > 1 else "unknown"

            key_hash = hashlib.md5(key.encode()).hexdigest()[:8]
            items.append({
                "id": f"{dataset}_{key_hash}",
                "source_path": key,
                "dataset": dataset,
                "size_bytes": obj["Size"],
            })

            datasets[dataset] = datasets.get(dataset, 0) + 1

            if len(items) >= args.count:
                break
        if len(items) >= args.count:
            break

    # Write manifest
    with open(manifest_path, "w") as f:
        json.dump(items, f, indent=2)

    manifest_hash = hashlib.sha256(manifest_path.read_bytes()).hexdigest()[:16]
    total_mb = sum(i["size_bytes"] for i in items) / 1_000_000

    print(f"\nWrote {manifest_path}")
    print(f"  {len(items)} items, {total_mb:.1f} MB total")
    print(f"  sha256: {manifest_hash}...")
    print(f"  Datasets:")
    for ds, count in sorted(datasets.items()):
        print(f"    {ds}: {count}")


if __name__ == "__main__":
    main()
