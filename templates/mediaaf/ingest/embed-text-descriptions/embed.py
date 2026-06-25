#!/usr/bin/env python3
# /// script
# requires-python = ">=3.11"
# dependencies = ["httpx", "python-dotenv"]
# ///
"""
Ingest media descriptions into Antfly via the HTTP API.

Replaces ingest/ingest_text.go — uses httpx directly instead of the Go SDK.
Creates a table with one vector index over combined_text. The media description
step happens before ingest; this script only stores rows and asks Antfly Cloud to
maintain text embeddings.

Usage:
    uv run ingest/embed-text-descriptions/embed.py --jsonl descriptions.jsonl
    uv run ingest/embed-text-descriptions/embed.py --source tgif
    uv run ingest/embed-text-descriptions/embed.py --all-sources
    uv run ingest/embed-text-descriptions/embed.py --jsonl descriptions.jsonl --limit 100
"""

import argparse
import hashlib
import json
import os
import sys
import time
from pathlib import Path

from dotenv import load_dotenv
import httpx

load_dotenv()

# Config
ANTFLY_URL = os.environ.get("ANTFLY_URL", "http://localhost:8080/api/v1")
TABLE_NAME = os.environ.get("INGEST_TABLE", "mediaaf")
BATCH_SIZE = 50
EMBED_MODEL = "BAAI/bge-small-en-v1.5"
EMBED_DIMENSION = 384
SOURCES_DIR = Path(__file__).parent.parent.parent / "sources"


def combined_text(desc: dict) -> str:
    """Create a searchable text blob from all description fields.

    Port of CombinedText() from ingest_text.go:85-95.
    """
    action = desc.get("action", "")
    if isinstance(action, list):
        action = ", ".join(action)

    names = desc.get("names", [])
    if isinstance(names, list):
        names = ", ".join(names)

    parts = [
        desc.get("literal", ""),
        "Source: " + desc.get("source", ""),
        "Mood: " + desc.get("mood", ""),
        "Actions: " + action,
        "Use case: " + desc.get("context", ""),
        "Tags: " + ", ".join(desc.get("tags", [])),
        "Names: " + names,
        "Visual style: " + desc.get("visual_style", ""),
        "Rating: " + desc.get("rating", ""),
    ]
    return ". ".join(parts)


def doc_id(desc: dict) -> str:
    """Generate document ID, preferring manifest ID if present.

    Port of DocID() from ingest_text.go:61-67.
    """
    if desc.get("id"):
        return desc["id"]
    h = hashlib.md5(desc["url"].encode()).hexdigest()[:16]
    return f"gif_{h}"


def create_table(client: httpx.Client, table: str) -> None:
    """Create Antfly table with one vector index over combined_text."""
    indexes = {
        "embeddings": {
            "type": "aknn_v0",
            "dimension": EMBED_DIMENSION,
            "field": "combined_text",
            "embedder": {
                "provider": "antfly",
                "model": EMBED_MODEL,
            },
        },
    }


    print(f"Creating table '{table}' with text embedding index (dim={EMBED_DIMENSION})...")

    body = {"indexes": indexes}

    resp = client.post(f"/tables/{table}", json=body)
    if resp.status_code == 409 or "already exists" in resp.text:
        print(f"Table '{table}' already exists, continuing...")
        return
    if not resp.is_success:
        print(f"Create table failed ({resp.status_code}): {resp.text}")
    resp.raise_for_status()
    print(f"Created table '{table}'")

    # Wait for shards to be ready
    wait_for_shards(client, table)
    print("Waiting 30s for shard stability...")
    time.sleep(30)


def wait_for_shards(client: httpx.Client, table: str, timeout: float = 30.0) -> None:
    """Poll until table shards are ready."""
    print("Waiting for shards to be ready...")
    deadline = time.time() + timeout
    polls = 0
    while time.time() < deadline:
        polls += 1
        time.sleep(0.5)
        try:
            resp = client.get(f"/tables/{table}")
            if resp.status_code == 200:
                data = resp.json()
                if data.get("shards") and polls >= 6:
                    print(f"Shards ready after {polls} polls")
                    return
        except httpx.HTTPError:
            continue
    raise TimeoutError("Timeout waiting for shards")


def flush_batch(client: httpx.Client, table: str, batch: dict) -> None:
    """Insert a batch of documents."""
    resp = client.post(
        f"/tables/{table}/batch",
        json={"inserts": batch},
        timeout=60.0,
    )
    resp.raise_for_status()


def build_doc(desc: dict, default_attribution: str, r2_urls: dict | None = None, media_base_url: str = "") -> dict:
    """Build an Antfly document from a description record."""
    url = desc.get("url", "")

    # Use R2 URL if available, or construct from source_path
    gif_url = url
    if r2_urls and url in r2_urls:
        gif_url = r2_urls[url]
    elif desc.get("r2_url"):
        gif_url = desc["r2_url"]
    elif desc.get("source_path") and media_base_url:
        gif_url = f"{media_base_url}/{desc['source_path']}"

    action = desc.get("action", "")
    if isinstance(action, list):
        action = ", ".join(action)

    # Pass through all description fields, skip metadata
    skip = {"id", "dataset", "source_path", "attribution"}
    doc = {k: v for k, v in desc.items() if k not in skip and v}
    doc["gif_url"] = gif_url
    doc["combined_text"] = combined_text(desc)

    attribution = desc.get("attribution", "") or default_attribution
    if attribution:
        doc["attribution"] = attribution

    return doc


def ingest_jsonl(client: httpx.Client, table: str, jsonl_path: str,
                 attribution: str, limit: int, r2_urls: dict | None = None,
                 media_base_url: str = "") -> int:
    """Ingest documents from a JSONL file. Returns count imported."""
    batch: dict = {}
    imported = 0
    start = time.time()

    print(f"Starting import from {jsonl_path}...")
    print(f"Model: {EMBED_MODEL}, Field: combined_text")

    with open(jsonl_path) as f:
        for line in f:
            desc = json.loads(line)
            did = doc_id(desc)
            doc = build_doc(desc, attribution, r2_urls, media_base_url)
            batch[did] = doc

            if len(batch) >= BATCH_SIZE:
                try:
                    flush_batch(client, table, batch)
                    imported += len(batch)
                except httpx.HTTPError as e:
                    print(f"\nWarning: batch insert failed ({len(batch)} docs lost): {e}", file=sys.stderr)
                batch = {}

                elapsed = time.time() - start
                rate = imported / elapsed if elapsed > 0 else 0
                print(f"\rImported: {imported} ({rate:.1f}/sec)", end="", flush=True)

                if limit and imported >= limit:
                    print(f"\nReached limit of {limit}")
                    break

    # Final batch
    if batch:
        try:
            flush_batch(client, table, batch)
            imported += len(batch)
        except httpx.HTTPError as e:
            print(f"\nWarning: final batch insert failed ({len(batch)} docs lost): {e}", file=sys.stderr)

    elapsed = time.time() - start
    rate = imported / elapsed if elapsed > 0 else 0
    print(f"\nCompleted: {imported} media items in {elapsed:.1f}s ({rate:.1f}/sec)")
    return imported


def find_sources(source_filter: str | None = None) -> list[Path]:
    """Find source directories with descriptions.jsonl."""
    sources = []
    if not SOURCES_DIR.exists():
        return sources
    for d in sorted(SOURCES_DIR.iterdir()):
        if not d.is_dir() or d.name.startswith("_"):
            continue
        if source_filter and d.name != source_filter:
            continue
        if (d / "descriptions.jsonl").exists():
            sources.append(d)
    return sources


def main():
    parser = argparse.ArgumentParser(description="Ingest media descriptions into Antfly")
    parser.add_argument("--jsonl", help="Path to descriptions JSONL file")
    parser.add_argument("--source", help="Ingest a specific source")
    parser.add_argument("--all-sources", action="store_true", help="Ingest all sources")
    parser.add_argument("--table", default=TABLE_NAME, help=f"Antfly table name (default: {TABLE_NAME})")
    parser.add_argument("--url", default=ANTFLY_URL, help=f"Antfly API URL (default: {ANTFLY_URL})")
    parser.add_argument("--attribution", default="", help="Default attribution for docs missing one")
    parser.add_argument("--limit", type=int, default=0, help="Limit number of docs to import (0=all)")
    parser.add_argument("--skip-create", action="store_true", help="Skip table creation")
    parser.add_argument("--batch-size", type=int, default=BATCH_SIZE, help=f"Batch size (default: {BATCH_SIZE})")
    parser.add_argument("--r2-urls", help="Path to R2 URL mapping JSON (from upload_r2.py)")
    parser.add_argument("--media-base-url", default="", help="Base URL for media (e.g., /media or https://cdn.example.com)")
    parser.add_argument("--token", default=os.environ.get("ANTFLYDB_API_KEY") or os.environ.get("ANTFLY_TOKEN") or "", help="Bearer token for Antfly Cloud auth")
    args = parser.parse_args()

    if not any([args.jsonl, args.source, args.all_sources]):
        parser.error("Specify --jsonl PATH, --source NAME, or --all-sources")

    headers = {"Authorization": f"Bearer {args.token}"} if args.token else {}
    client = httpx.Client(base_url=args.url, headers=headers, timeout=30.0)

    # Create table (unless skipped or ingesting additional sources into existing table)
    if not args.skip_create:
        create_table(client, args.table)

    # Load R2 URL mapping if provided
    r2_urls = None
    if args.r2_urls:
        with open(args.r2_urls) as f:
            r2_urls = json.load(f)
        print(f"Loaded {len(r2_urls)} R2 URL mappings")

    if args.jsonl:
        ingest_jsonl(client, args.table, args.jsonl, args.attribution, args.limit, r2_urls, args.media_base_url)

    if args.source:
        sources = find_sources(args.source)
        if not sources:
            print(f"Error: no descriptions.jsonl found for source '{args.source}'", file=sys.stderr)
            sys.exit(1)
        for source_dir in sources:
            jsonl_path = str(source_dir / "descriptions.jsonl")
            print(f"\n=== {source_dir.name} ===")
            ingest_jsonl(client, args.table, jsonl_path, args.attribution, args.limit, r2_urls, args.media_base_url)

    if args.all_sources:
        sources = find_sources()
        if not sources:
            print("No sources with descriptions found")
            return
        print(f"Ingesting {len(sources)} sources: {', '.join(s.name for s in sources)}")
        total = 0
        for source_dir in sources:
            jsonl_path = str(source_dir / "descriptions.jsonl")
            print(f"\n=== {source_dir.name} ===")
            total += ingest_jsonl(client, args.table, jsonl_path, args.attribution, args.limit, r2_urls, args.media_base_url)
        print(f"\nTotal ingested: {total}")


if __name__ == "__main__":
    main()
