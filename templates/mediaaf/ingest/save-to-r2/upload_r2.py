#!/usr/bin/env python3
# /// script
# requires-python = ">=3.11"
# dependencies = ["boto3", "python-dotenv"]
# ///
"""
Upload source media to Cloudflare R2 storage.

For each source, resolves the manifest in priority order:
  1. Local: sources/{name}/manifest.json
  2. Remote: R2 at sources/{name}/manifest.json
  3. Not found: run the source's scraper first

For each item without an r2_url, streams directly from original_url to R2.
Periodically syncs the manifest back to R2 for durability.

Bucket path structure:
  sources/{source_name}/manifest.json
  sources/{source_name}/media/{item_id}.{ext}

Usage:
    uv run ingest/save-to-r2/upload_r2.py --source tgif
    uv run ingest/save-to-r2/upload_r2.py --source tgif
    uv run ingest/save-to-r2/upload_r2.py --source tgif --workers 20
    uv run ingest/save-to-r2/upload_r2.py --all-sources
"""

import argparse
import json
import os
import sys
import threading
import time
from concurrent.futures import ThreadPoolExecutor, as_completed
from pathlib import Path
from urllib.request import urlopen, Request

from dotenv import load_dotenv
import boto3
from botocore.config import Config as BotoConfig
from botocore.exceptions import ClientError

load_dotenv()

# Config
SOURCES_DIR = Path(__file__).parent.parent.parent / "sources"
BUCKET_NAME = "honeycomb-media"
MANIFEST_SYNC_INTERVAL = 100  # sync manifest to R2 every N completions
DEFAULT_WORKERS = 10


def get_s3_client():
    """Create S3-compatible client for Cloudflare R2."""
    return boto3.client(
        "s3",
        endpoint_url=os.environ["R2_ENDPOINT_URL"],
        aws_access_key_id=os.environ["R2_ACCESS_KEY_ID"],
        aws_secret_access_key=os.environ["R2_SECRET_ACCESS_KEY"],
        config=BotoConfig(
            connect_timeout=30,
            read_timeout=120,
            retries={"max_attempts": 3, "mode": "adaptive"},
        ),
    )


def get_public_url(key: str) -> str:
    """Get public CDN URL for an R2 object."""
    base = os.environ.get("R2_PUBLIC_URL", "https://media.example.com")
    return f"{base.rstrip('/')}/{key}"


def object_exists(s3, key: str) -> bool:
    """Check if an object already exists in R2."""
    try:
        s3.head_object(Bucket=BUCKET_NAME, Key=key)
        return True
    except ClientError as e:
        if e.response["Error"]["Code"] == "404":
            return False
        raise


def upload_bytes(s3, key: str, data: bytes, content_type: str = "image/gif") -> str:
    """Upload data to R2 and return the public URL."""
    s3.put_object(
        Bucket=BUCKET_NAME,
        Key=key,
        Body=data,
        ContentType=content_type,
    )
    return get_public_url(key)


def download_url(url: str, timeout: int = 30) -> bytes | None:
    """Download a file from URL and return bytes."""
    try:
        req = Request(url, headers={"User-Agent": "Mozilla/5.0"})
        with urlopen(req, timeout=timeout) as resp:
            # Detect Tumblr removal redirects
            if "assets.tumblr.com/images/media_violation/" in resp.url:
                return None
            return resp.read()
    except Exception as e:
        print(f"  Download failed for {url[:80]}: {e}", file=sys.stderr)
        return None


def content_type_for(ext: str) -> str:
    """Return MIME type for a file extension."""
    if ext in ("mp4", "webm"):
        return f"video/{ext}"
    return f"image/{ext}"


# --- Manifest resolution ---

def resolve_manifest(s3, source_name: str) -> dict | None:
    """Resolve manifest: local first, then R2, else None."""
    local_dir = SOURCES_DIR / source_name
    local_path = local_dir / "manifest.json"

    # 1. Local manifest
    if local_path.exists():
        print(f"  Using local manifest: {local_path}")
        with open(local_path) as f:
            return json.load(f)

    # 2. Remote manifest from R2
    r2_key = f"sources/{source_name}/manifest.json"
    try:
        resp = s3.get_object(Bucket=BUCKET_NAME, Key=r2_key)
        manifest = json.loads(resp["Body"].read())
        print(f"  Pulled manifest from R2: {r2_key}")
        # Save locally for future runs
        local_dir.mkdir(parents=True, exist_ok=True)
        with open(local_path, "w") as f:
            json.dump(manifest, f, indent=2)
        return manifest
    except ClientError as e:
        if e.response["Error"]["Code"] == "NoSuchKey":
            return None
        raise


def save_manifest_local(source_name: str, manifest: dict) -> None:
    """Save manifest to local disk."""
    source_dir = SOURCES_DIR / source_name
    source_dir.mkdir(parents=True, exist_ok=True)
    tmp = (source_dir / "manifest.json").with_suffix(".tmp")
    with open(tmp, "w") as f:
        json.dump(manifest, f, indent=2)
    tmp.rename(source_dir / "manifest.json")


def save_manifest_r2(s3, source_name: str, manifest: dict) -> None:
    """Sync manifest to R2."""
    key = f"sources/{source_name}/manifest.json"
    s3.put_object(
        Bucket=BUCKET_NAME,
        Key=key,
        Body=json.dumps(manifest, indent=2).encode(),
        ContentType="application/json",
    )


# --- Upload ---

def process_item(s3, source_name: str, item: dict) -> str:
    """Process a single item: download + upload. Returns 'uploaded', 'skipped', or 'failed'.

    On success, sets item['r2_url'] as a side effect.
    """
    item_id = item["id"]
    fmt = item.get("format", "gif")
    ext = fmt if fmt in ("gif", "mp4", "webm") else "gif"
    key = f"sources/{source_name}/media/{item_id}.{ext}"

    # Check R2 directly (in case manifest is stale)
    if object_exists(s3, key):
        item["r2_url"] = get_public_url(key)
        return "skipped"

    # Try local cache first, then stream from original URL
    data = None
    local_file = item.get("local_file")
    if local_file:
        local_path = SOURCES_DIR / source_name / local_file
        if local_path.exists():
            data = local_path.read_bytes()

    if data is None:
        url = item.get("original_url", "")
        if not url:
            return "failed"
        data = download_url(url)
        if data is None:
            return "failed"

    r2_url = upload_bytes(s3, key, data, content_type_for(ext))
    item["r2_url"] = r2_url
    return "uploaded"


def upload_source(s3, source_name: str, workers: int = DEFAULT_WORKERS) -> int:
    """Upload media for a source using a thread pool. Returns count uploaded."""
    manifest = resolve_manifest(s3, source_name)
    if manifest is None:
        print(f"  No manifest found for {source_name} (local or R2).")
        print(f"  Run the scraper first: uv run sources/{source_name}/scrape.py")
        return 0

    items = manifest.get("items", [])
    if not items:
        print(f"  No items in manifest for {source_name}")
        return 0

    # Count what needs uploading
    to_upload = [i for i in items if not i.get("r2_url")]
    if not to_upload:
        print(f"  All {len(items)} items already uploaded")
        return 0

    previously_done = len(items) - len(to_upload)
    print(f"  {len(to_upload)} to upload ({previously_done} already done, {len(items)} total)")
    print(f"  Using {workers} workers")

    uploaded = 0
    skipped = 0
    failed = 0
    lock = threading.Lock()
    start = time.time()
    since_last_sync = 0

    def on_result(result: str):
        nonlocal uploaded, skipped, failed, since_last_sync
        if result == "uploaded":
            uploaded += 1
            since_last_sync += 1
        elif result == "skipped":
            skipped += 1
            since_last_sync += 1
        else:
            failed += 1

    # Each thread gets its own S3 client to avoid connection pool contention
    thread_local = threading.local()

    def get_thread_s3():
        if not hasattr(thread_local, "s3"):
            thread_local.s3 = get_s3_client()
        return thread_local.s3

    def worker(item):
        s3_t = get_thread_s3()
        try:
            return process_item(s3_t, source_name, item)
        except Exception as e:
            print(f"\n  Error uploading {item.get('id', '?')}: {e}", file=sys.stderr)
            return "failed"

    with ThreadPoolExecutor(max_workers=workers) as pool:
        futures = {pool.submit(worker, item): item for item in to_upload}

        for future in as_completed(futures):
            result = future.result()
            with lock:
                on_result(result)

                done = uploaded + skipped + failed
                total_on_r2 = previously_done + uploaded + skipped

                # Periodic local save for crash recovery
                if since_last_sync >= MANIFEST_SYNC_INTERVAL:
                    save_manifest_local(source_name, manifest)
                    since_last_sync = 0

                if done % 25 == 0 or done == len(to_upload):
                    elapsed = time.time() - start
                    rate = done / elapsed if elapsed > 0 else 0
                    fail_str = f", {failed} failed" if failed else ""
                    print(f"\r  [{source_name}] {total_on_r2}/{len(items)} on R2 "
                          f"(+{uploaded + skipped} this run{fail_str}) {rate:.1f}/sec",
                          end="", flush=True)

    # Final sync
    save_manifest_local(source_name, manifest)
    try:
        save_manifest_r2(s3, source_name, manifest)
    except Exception as e:
        print(f"\n  Warning: final R2 manifest sync failed ({e}), local saved", file=sys.stderr)
    print()
    return uploaded


def find_sources() -> list[str]:
    """Find all source directories with a manifest.json (local only)."""
    sources = []
    if not SOURCES_DIR.exists():
        return sources
    for d in sorted(SOURCES_DIR.iterdir()):
        if d.is_dir() and not d.name.startswith("_") and (d / "manifest.json").exists():
            sources.append(d.name)
    return sources


def main():
    parser = argparse.ArgumentParser(description="Upload source media to Cloudflare R2")
    parser.add_argument("--source", help="Upload media for a specific source")
    parser.add_argument("--all-sources", action="store_true", help="Upload all sources with manifests")
    parser.add_argument("--workers", type=int, default=DEFAULT_WORKERS, help=f"Concurrent uploads (default {DEFAULT_WORKERS})")
    args = parser.parse_args()

    if not any([args.source, args.all_sources]):
        parser.error("Specify --source NAME or --all-sources")

    # Validate env vars
    for var in ["R2_ENDPOINT_URL", "R2_ACCESS_KEY_ID", "R2_SECRET_ACCESS_KEY"]:
        if var not in os.environ:
            print(f"Error: {var} environment variable not set", file=sys.stderr)
            sys.exit(1)

    s3 = get_s3_client()

    if args.source:
        print(f"Uploading source: {args.source}")
        count = upload_source(s3, args.source, workers=args.workers)
        print(f"Uploaded {count} files for {args.source}")

    if args.all_sources:
        sources = find_sources()
        if not sources:
            print("No sources found (check local sources/ directory)")
            return
        print(f"Uploading {len(sources)} sources: {', '.join(sources)}")
        total = 0
        for name in sources:
            print(f"\n=== {name} ===")
            total += upload_source(s3, name, workers=args.workers)
        print(f"\nTotal uploaded: {total}")


if __name__ == "__main__":
    main()
