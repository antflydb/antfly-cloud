#!/usr/bin/env python3
# /// script
# requires-python = ">=3.11"
# dependencies = ["httpx"]
# ///
"""
Delete all documents from an Antfly table whose IDs match a given prefix.

First does a dry-run count, then asks for confirmation before deleting.

Usage:
    uv run scripts/delete_by_prefix.py km_
    uv run scripts/delete_by_prefix.py km_ --table honeycomb
    uv run scripts/delete_by_prefix.py km_ --url http://remote:8080/api/v1
    uv run scripts/delete_by_prefix.py km_ --yes  # skip confirmation
"""

import argparse
import sys

import httpx

DEFAULT_URL = "http://localhost:8080/api/v1"
DEFAULT_TABLE = "honeycomb"
SCAN_LIMIT = 500
DELETE_BATCH_SIZE = 100


def find_ids_by_prefix(client: httpx.Client, table: str, prefix: str) -> list[str]:
    """Scan the table for all document IDs matching the prefix."""
    matched: list[str] = []
    offset = 0

    while True:
        resp = client.post(
            f"/tables/{table}/query",
            json={"limit": SCAN_LIMIT, "offset": offset},
            timeout=30.0,
        )
        resp.raise_for_status()
        data = resp.json()

        hits = data.get("responses", [{}])[0].get("hits", {}).get("hits", [])
        if not hits:
            break

        for hit in hits:
            doc_id = hit.get("id") or hit.get("_id", "")
            if doc_id.startswith(prefix):
                matched.append(doc_id)

        offset += len(hits)
        total = data.get("responses", [{}])[0].get("hits", {}).get("total", 0)
        print(f"\r  Scanned {offset}/{total} documents, found {len(matched)} matching '{prefix}'", end="", flush=True)

        if offset >= total:
            break

    print()
    return matched


def delete_ids(client: httpx.Client, table: str, ids: list[str]) -> int:
    """Delete documents by ID in batches. Returns count deleted."""
    deleted = 0
    for i in range(0, len(ids), DELETE_BATCH_SIZE):
        batch = ids[i : i + DELETE_BATCH_SIZE]
        resp = client.post(
            f"/tables/{table}/batch",
            json={"deletes": batch},
            timeout=30.0,
        )
        resp.raise_for_status()
        deleted += len(batch)
        print(f"\r  Deleted {deleted}/{len(ids)}", end="", flush=True)

    print()
    return deleted


def main():
    parser = argparse.ArgumentParser(description="Delete Antfly documents by ID prefix")
    parser.add_argument("prefix", help="ID prefix to match (e.g., 'km_')")
    parser.add_argument("--table", default=DEFAULT_TABLE, help=f"Table name (default: {DEFAULT_TABLE})")
    parser.add_argument("--url", default=DEFAULT_URL, help=f"Antfly API URL (default: {DEFAULT_URL})")
    parser.add_argument("--yes", "-y", action="store_true", help="Skip confirmation prompt")
    args = parser.parse_args()

    client = httpx.Client(base_url=args.url, timeout=30.0)

    print(f"Scanning table '{args.table}' for IDs starting with '{args.prefix}'...")
    ids = find_ids_by_prefix(client, args.table, args.prefix)

    if not ids:
        print("No matching documents found.")
        return

    print(f"\nFound {len(ids)} documents to delete.")
    if ids[:5]:
        print(f"  Sample IDs: {', '.join(ids[:5])}")

    if not args.yes:
        confirm = input(f"\nDelete {len(ids)} documents from '{args.table}'? [y/N] ")
        if confirm.lower() != "y":
            print("Aborted.")
            sys.exit(0)

    print(f"\nDeleting {len(ids)} documents...")
    deleted = delete_ids(client, args.table, ids)
    print(f"Done. Deleted {deleted} documents from '{args.table}'.")


if __name__ == "__main__":
    main()
