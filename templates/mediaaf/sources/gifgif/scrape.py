#!/usr/bin/env python3
# /// script
# requires-python = ">=3.11"
# dependencies = ["boto3", "python-dotenv"]
# ///
"""
Build manifest from the MIT Media Lab GifGif dataset.

Unlike other scrapers, this one combines scrape + upload: it streams the
5.3 GB tar archive from S3, uploading each GIF directly to R2 without
writing anything large to disk. The emotion comparison CSV (~52 MB) is
also downloaded and parsed to compute per-GIF emotion scores.

The dataset contains ~6170 GIFs with 2.7M+ pairwise comparisons across
17 emotions: amusement, anger, contentment, disgust, embarrassment,
excitement, fear, guilt, happiness, pleasure, pride, relief, sadness,
satisfaction, shame, surprise, sympathy.

Source: http://lucas.maystre.ch/gifgif-data

Requires R2 credentials in .env (same as pipeline/upload_r2.py).

Usage:
    uv run sources/gifgif/scrape.py
    uv run sources/gifgif/scrape.py --limit 100
    uv run sources/gifgif/scrape.py --force
"""

import argparse
import csv
import gzip
import json
import os
import sys
import tarfile
import time
from collections import defaultdict
from datetime import datetime, timezone
from io import BytesIO
from pathlib import Path
from urllib.request import urlopen, Request

from dotenv import load_dotenv
import boto3
from botocore.exceptions import ClientError

load_dotenv()

SOURCE_NAME = "gifgif"
SOURCE_DIR = Path(__file__).parent
MANIFEST_PATH = SOURCE_DIR / "manifest.json"

CSV_URL = "https://s3-eu-west-1.amazonaws.com/lum-public/gifgif-dataset-20150121-v1.csv.gz"
TAR_URL = "https://s3-eu-west-1.amazonaws.com/lum-public/gifgif-images-v1.tar"
SOURCE_PAGE = "http://lucas.maystre.ch/gifgif-data"

BUCKET_NAME = os.environ.get("R2_BUCKET", "mediaaf-media")
MANIFEST_SYNC_INTERVAL = 50


def get_s3_client():
    return boto3.client(
        "s3",
        endpoint_url=os.environ["R2_ENDPOINT_URL"],
        aws_access_key_id=os.environ["R2_ACCESS_KEY_ID"],
        aws_secret_access_key=os.environ["R2_SECRET_ACCESS_KEY"],
    )


def get_public_url(key: str) -> str:
    base = os.environ.get("R2_PUBLIC_URL", "https://media.example.com")
    return f"{base.rstrip('/')}/{key}"


def object_exists(s3, key: str) -> bool:
    try:
        s3.head_object(Bucket=BUCKET_NAME, Key=key)
        return True
    except ClientError as e:
        if e.response["Error"]["Code"] == "404":
            return False
        raise


def load_manifest() -> dict:
    if MANIFEST_PATH.exists():
        with open(MANIFEST_PATH) as f:
            return json.load(f)
    return {
        "source": SOURCE_NAME,
        "source_url": SOURCE_PAGE,
        "scraped_at": None,
        "items": [],
    }


def save_manifest(manifest: dict) -> None:
    tmp = MANIFEST_PATH.with_suffix(".tmp")
    with open(tmp, "w") as f:
        json.dump(manifest, f, indent=2)
    tmp.rename(MANIFEST_PATH)


def save_manifest_r2(s3, manifest: dict) -> None:
    key = f"sources/{SOURCE_NAME}/manifest.json"
    s3.put_object(
        Bucket=BUCKET_NAME,
        Key=key,
        Body=json.dumps(manifest, indent=2).encode(),
        ContentType="application/json",
    )


def fetch_emotion_scores() -> dict[str, dict]:
    """Download the emotion CSV and compute per-GIF scores.

    Returns gif_id -> {"emotions": {emotion: score}, "top_emotions": [(emotion, score), ...]}
    where score = wins / total_comparisons for that emotion.
    """
    print("Downloading emotion comparison data...")
    req = Request(CSV_URL, headers={"User-Agent": "Mozilla/5.0"})

    with urlopen(req) as response:
        compressed = BytesIO(response.read())

    wins = defaultdict(lambda: defaultdict(int))
    comparisons = defaultdict(lambda: defaultdict(int))

    with gzip.open(compressed, "rt") as f:
        reader = csv.DictReader(f)
        row_count = 0
        for row in reader:
            metric = row["metric"]
            left = row["left"]
            right = row["right"]
            choice = row["choice"]

            comparisons[left][metric] += 1
            comparisons[right][metric] += 1

            if choice == "left":
                wins[left][metric] += 1
            elif choice == "right":
                wins[right][metric] += 1

            row_count += 1
            if row_count % 500_000 == 0:
                print(f"  Processed {row_count:,} comparisons...")

    print(f"  {row_count:,} comparisons across {len(comparisons):,} GIFs")

    scores = {}
    for gif_id in comparisons:
        gif_scores = {}
        for emotion in comparisons[gif_id]:
            total = comparisons[gif_id][emotion]
            w = wins.get(gif_id, {}).get(emotion, 0)
            if total > 0:
                gif_scores[emotion] = round(w / total, 3)

        top = sorted(gif_scores.items(), key=lambda x: -x[1])
        scores[gif_id] = {
            "emotions": gif_scores,
            "top_emotions": top[:5],
        }

    return scores


def main():
    parser = argparse.ArgumentParser(description="Build manifest from GifGif dataset")
    parser.add_argument("--limit", type=int, default=0, help="Limit items (0=all)")
    parser.add_argument("--force", action="store_true", help="Clear manifest and re-process")
    args = parser.parse_args()

    # Validate R2 credentials
    for var in ["R2_ENDPOINT_URL", "R2_ACCESS_KEY_ID", "R2_SECRET_ACCESS_KEY"]:
        if var not in os.environ:
            print(f"Error: {var} not set", file=sys.stderr)
            sys.exit(1)

    manifest = load_manifest()

    if args.force:
        manifest["items"] = []
        manifest["scraped_at"] = None
    elif manifest["scraped_at"] and manifest["items"]:
        print(f"Manifest already has {len(manifest['items'])} items (use --force to redo)")
        return

    existing_ids = {item["id"] for item in manifest["items"]}

    # Step 1: Emotion scores from CSV
    emotion_scores = fetch_emotion_scores()

    # Step 2: Stream tar, upload each GIF to R2
    s3 = get_s3_client()

    print(f"Streaming GIF archive ({TAR_URL})...")
    req = Request(TAR_URL, headers={"User-Agent": "Mozilla/5.0"})

    added = 0
    uploaded = 0
    skipped = 0
    failed = 0
    start = time.time()
    since_last_sync = 0

    with urlopen(req) as response:
        with tarfile.open(fileobj=response, mode="r|") as tar:
            for member in tar:
                if not member.isfile() or not member.name.endswith(".gif"):
                    continue

                gif_id_raw = Path(member.name).stem
                item_id = f"gg_{gif_id_raw}"

                if item_id in existing_ids:
                    continue

                # Emotion metadata
                emo = emotion_scores.get(gif_id_raw, {})
                top_emotions = emo.get("top_emotions", [])
                emotions_dict = {e: s for e, s in top_emotions} if top_emotions else {}

                mood = top_emotions[0][0] if top_emotions else ""
                strong = [e for e, s in top_emotions if s > 0.3]
                title = ", ".join(strong) if strong else (mood or "animated gif")

                item = {
                    "id": item_id,
                    "original_url": "",
                    "page_url": SOURCE_PAGE,
                    "title": title,
                    "format": "gif",
                    "attribution": "MIT Media Lab GifGif",
                }
                if emotions_dict:
                    item["emotions"] = emotions_dict

                # Upload to R2
                key = f"sources/{SOURCE_NAME}/media/{item_id}.gif"
                if object_exists(s3, key):
                    item["r2_url"] = get_public_url(key)
                    skipped += 1
                else:
                    try:
                        f = tar.extractfile(member)
                        data = f.read()
                        s3.put_object(
                            Bucket=BUCKET_NAME,
                            Key=key,
                            Body=data,
                            ContentType="image/gif",
                        )
                        item["r2_url"] = get_public_url(key)
                        uploaded += 1
                    except Exception as e:
                        print(f"\n  Upload failed for {item_id}: {e}", file=sys.stderr)
                        failed += 1

                since_last_sync += 1
                manifest["items"].append(item)
                existing_ids.add(item_id)
                added += 1

                if since_last_sync >= MANIFEST_SYNC_INTERVAL:
                    save_manifest(manifest)
                    save_manifest_r2(s3, manifest)
                    since_last_sync = 0

                if added % 10 == 0:
                    elapsed = time.time() - start
                    rate = added / elapsed if elapsed > 0 else 0
                    print(
                        f"\r  {added} found, {uploaded} uploaded, {skipped} on R2, "
                        f"{failed} failed ({rate:.1f}/sec)",
                        end="",
                        flush=True,
                    )

                if args.limit and added >= args.limit:
                    break

    manifest["scraped_at"] = datetime.now(timezone.utc).isoformat()
    save_manifest(manifest)
    save_manifest_r2(s3, manifest)

    print(f"\nDone: {added} items added ({len(manifest['items'])} total)")
    print(f"  {uploaded} uploaded, {skipped} already on R2, {failed} failed")


if __name__ == "__main__":
    main()
