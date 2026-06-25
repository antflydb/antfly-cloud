#!/usr/bin/env python3
# /// script
# requires-python = ">=3.11"
# dependencies = []
# ///
"""
Build a manifest from the TGIF dataset TSV file.

Reads the TSV (URL + description per line), generates manifest items in the
standard format. Does NOT download media — that's handled by
pipeline/upload_r2.py which streams directly to R2.

If the TSV isn't found locally, it's downloaded automatically from GitHub.

Usage:
    uv run sources/tgif/scrape.py
    uv run sources/tgif/scrape.py --limit 100
    uv run sources/tgif/scrape.py --tsv path/to/tgif-v1.0.tsv
"""

import argparse
import hashlib
import json
import os
from datetime import datetime, timezone
from pathlib import Path
from urllib.request import urlopen, Request

SOURCE_NAME = "tgif"
SOURCE_DIR = Path(__file__).parent
MANIFEST_PATH = SOURCE_DIR / "manifest.json"
DEFAULT_TSV = SOURCE_DIR / "tgif-v1.0.tsv"
TSV_URL = "https://raw.githubusercontent.com/raingo/TGIF-Release/master/data/tgif-v1.0.tsv"


def fix_tumblr_url(url: str) -> str:
    """Update old Tumblr CDN URLs to new domain."""
    for old in ["38.media", "33.media", "31.media"]:
        url = url.replace(f"{old}.tumblr.com", "64.media.tumblr.com")
    return url


def make_id(url: str) -> str:
    """Generate deterministic ID from URL."""
    h = hashlib.md5(url.encode()).hexdigest()[:16]
    return f"gif_{h}"


def load_manifest() -> dict:
    if MANIFEST_PATH.exists():
        with open(MANIFEST_PATH) as f:
            return json.load(f)
    return {
        "source": SOURCE_NAME,
        "source_url": "https://github.com/raingo/TGIF-Release",
        "scraped_at": None,
        "items": [],
    }


def save_manifest(manifest: dict) -> None:
    tmp = MANIFEST_PATH.with_suffix(".tmp")
    with open(tmp, "w") as f:
        json.dump(manifest, f, indent=2)
    tmp.rename(MANIFEST_PATH)


def main():
    parser = argparse.ArgumentParser(description="Build manifest from TGIF dataset")
    parser.add_argument("--tsv", default=DEFAULT_TSV, help="Path to tgif-v1.0.tsv")
    parser.add_argument("--limit", type=int, default=0, help="Limit items (0=all)")
    parser.add_argument("--force", action="store_true", help="Re-read TSV even if manifest exists")
    args = parser.parse_args()

    tsv_path = Path(args.tsv)

    if not tsv_path.exists():
        print(f"TSV not found at {tsv_path}, downloading from GitHub...")
        req = Request(TSV_URL, headers={"User-Agent": "Mozilla/5.0"})
        with urlopen(req) as resp:
            tsv_path.parent.mkdir(parents=True, exist_ok=True)
            tsv_path.write_bytes(resp.read())
        print(f"  Saved to {tsv_path}")

    manifest = load_manifest()
    existing_urls = {item["original_url"] for item in manifest["items"]}

    if not args.force and manifest["scraped_at"]:
        print(f"Manifest already exists with {len(manifest['items'])} items (use --force to re-read)")
        return

    print(f"Reading TGIF dataset from {tsv_path}...")
    added = 0
    with open(tsv_path) as f:
        for line in f:
            parts = line.strip().split("\t", 1)
            if len(parts) != 2:
                continue

            url = fix_tumblr_url(parts[0])
            description = parts[1]

            if url in existing_urls:
                continue

            item_id = make_id(url)
            item = {
                "id": item_id,
                "original_url": url,
                "page_url": "",
                "title": description,
                "format": "gif",
                "attribution": "TGIF dataset",
            }
            manifest["items"].append(item)
            existing_urls.add(url)
            added += 1

            if args.limit and added >= args.limit:
                break

    manifest["scraped_at"] = datetime.now(timezone.utc).isoformat()
    save_manifest(manifest)
    print(f"Added {added} items to manifest ({len(manifest['items'])} total)")


if __name__ == "__main__":
    main()
