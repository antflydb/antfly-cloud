#!/usr/bin/env python3
# /// script
# requires-python = ">=3.11"
# dependencies = ["google-genai", "Pillow"]
# ///
"""
Generate rich text descriptions of GIFs using Gemini 2.0 Flash Lite.
These descriptions can then be embedded with text-only models for search.

Usage:
    uv run describe_gifs.py --limit 100             # Test run on 100 GIFs
    uv run describe_gifs.py --limit 0 --resume      # Process all GIFs (resumable)
    uv run describe_gifs.py --limit 0 --workers 30  # Faster with more workers
"""

import argparse
import io
import json
import os
import sys
import threading
import time
from concurrent.futures import ThreadPoolExecutor, as_completed
from pathlib import Path
from urllib.request import urlopen, Request

from PIL import Image
from google import genai
from google.genai import types

# Config
DEFAULT_TSV = "../datasets/TGIF-Release/data/tgif-v1.0.tsv"
OUTPUT_FILE = "gif_descriptions.jsonl"
API_KEY_PATH = os.path.expanduser("~/.tokens/gemini_api_key")

# Gemini model - Flash Lite is cheapest
MODEL_NAME = "gemini-2.0-flash-lite"

# Prompt for generating multiple description types
DESCRIPTION_PROMPT = """You are analyzing multiple frames extracted from an animated GIF. The frames are shown in chronological order.

Analyze the FULL sequence of action across all frames and return a JSON object:
- "literal": Factual description of the complete action/sequence (1-3 sentences, describe what happens from start to finish)
- "source": Your best guess at where this is from — movie title, TV show, meme name, video game, news event, YouTube/TikTok trend, etc. Be specific (e.g., "Spy Kids (2001)" not just "movie"). Use "unknown" only if you genuinely cannot identify it.
- "mood": Emotional tone or vibe (e.g., "funny", "wholesome", "chaotic", "satisfying")
- "action": Key actions/verbs (e.g., "dancing", "falling", "celebrating")
- "context": When someone might use this GIF in conversation (e.g., "reaction to good news")
- "tags": Array of 5-10 searchable keywords (include character names, show titles, meme names if recognized)

Respond with ONLY the JSON object, no markdown or extra text."""

# Frame extraction settings
NUM_FRAMES = 5
MAX_FRAME_DIM = 512


def load_api_key() -> str:
    """Load Gemini API key from file."""
    with open(API_KEY_PATH) as f:
        return f.read().strip().split()[0]  # First token, ignore comments


def fix_tumblr_url(url: str) -> str:
    """Update old Tumblr CDN URLs to new domain."""
    for old in ["38.media", "33.media", "31.media"]:
        url = url.replace(f"{old}.tumblr.com", "64.media.tumblr.com")
    return url


def load_gifs(tsv_path: str, limit: int = 0) -> list[dict]:
    """Load GIFs from TGIF TSV file."""
    gifs = []
    with open(tsv_path) as f:
        for line in f:
            parts = line.strip().split("\t", 1)
            if len(parts) != 2:
                continue
            url, desc = parts
            gifs.append({
                "url": fix_tumblr_url(url),
                "original_description": desc
            })
            if limit and len(gifs) >= limit:
                break
    return gifs


def download_gif(url: str, timeout: int = 30) -> bytes | None:
    """Download a GIF from URL and return bytes."""
    try:
        req = Request(url, headers={"User-Agent": "Mozilla/5.0"})
        with urlopen(req, timeout=timeout) as resp:
            # Detect Tumblr removal redirects (copyright/guideline violations)
            if "assets.tumblr.com/images/media_violation/" in resp.url:
                print(f"  Skipping removed GIF: {url}", file=sys.stderr)
                return None
            return resp.read()
    except Exception as e:
        print(f"  Download error: {e}", file=sys.stderr)
        return None


def extract_frames(gif_data: bytes, num_frames: int = NUM_FRAMES, max_dim: int = MAX_FRAME_DIM) -> list[bytes]:
    """Extract evenly-spaced frames from a GIF as PNG bytes, resizing if needed."""
    img = Image.open(io.BytesIO(gif_data))
    n_frames = getattr(img, "n_frames", 1)

    # Pick frame indices: evenly spaced including first and last
    if n_frames <= num_frames:
        indices = list(range(n_frames))
    else:
        indices = [round(i * (n_frames - 1) / (num_frames - 1)) for i in range(num_frames)]

    frames = []
    for idx in indices:
        img.seek(idx)
        frame = img.convert("RGBA")

        # Resize if too large
        w, h = frame.size
        if max(w, h) > max_dim:
            scale = max_dim / max(w, h)
            frame = frame.resize((round(w * scale), round(h * scale)), Image.LANCZOS)

        buf = io.BytesIO()
        frame.save(buf, format="PNG")
        frames.append(buf.getvalue())

    return frames


def _clean_json_text(text: str) -> str:
    """Strip markdown fences and trailing garbage from Gemini JSON responses."""
    text = text.strip()
    if text.startswith("```"):
        lines = text.split("\n")
        text = "\n".join(lines[1:-1])
    return text


def describe_gif_from_data(client, gif_data: bytes, gif_url: str, retries: int = 1) -> dict | None:
    """Generate descriptions for a GIF from its raw bytes using Gemini."""
    try:
        # Extract key frames
        frames = extract_frames(gif_data)

        # Build parts: prompt + each frame as a PNG
        parts = [types.Part.from_text(text=DESCRIPTION_PROMPT)]
        for frame_png in frames:
            parts.append(types.Part.from_bytes(data=frame_png, mime_type="image/png"))

        for attempt in range(1 + retries):
            try:
                response = client.models.generate_content(
                    model=MODEL_NAME,
                    contents=[types.Content(parts=parts)]
                )
                text = _clean_json_text(response.text)
                return json.loads(text)
            except json.JSONDecodeError as e:
                if attempt < retries:
                    print(f"  JSON parse error (retrying): {e}", file=sys.stderr)
                else:
                    print(f"  JSON parse error (giving up): {e}", file=sys.stderr)
                    print(f"    Raw response: {response.text[:300]}", file=sys.stderr)

        return None
    except Exception as e:
        print(f"  API error: {e}", file=sys.stderr)
        return None


# Sentinel to distinguish "removed GIF" from "API/parse failure"
REMOVED = "REMOVED"


def process_gif(client, gif: dict) -> dict | str | None:
    """Process a single GIF. Returns dict on success, REMOVED for skipped GIFs, None on failure."""
    gif_data = download_gif(gif["url"])
    if gif_data is None:
        return REMOVED

    descriptions = describe_gif_from_data(client, gif_data, gif["url"])
    if descriptions:
        return {
            "url": gif["url"],
            "original_description": gif["original_description"],
            **descriptions
        }
    return None


def main():
    parser = argparse.ArgumentParser(description="Generate GIF descriptions with Gemini")
    parser.add_argument("--tsv", default=DEFAULT_TSV, help="Path to TGIF TSV file")
    parser.add_argument("--limit", type=int, default=100, help="Limit GIFs to process (0=all)")
    parser.add_argument("--output", default=OUTPUT_FILE, help="Output JSONL file")
    parser.add_argument("--resume", action="store_true", help="Resume from existing output")
    parser.add_argument("--workers", type=int, default=20, help="Number of concurrent workers (default: 20)")
    args = parser.parse_args()

    # Setup Gemini client (new API)
    api_key = load_api_key()
    client = genai.Client(api_key=api_key)

    # Load GIFs
    print(f"Loading GIFs from {args.tsv}...")
    gifs = load_gifs(args.tsv, args.limit)
    print(f"Loaded {len(gifs)} GIFs")

    # Check for existing progress
    processed_urls = set()
    if args.resume and Path(args.output).exists():
        with open(args.output) as f:
            for line in f:
                data = json.loads(line)
                processed_urls.add(data["url"])
        print(f"Resuming: {len(processed_urls)} already processed")

    # Filter out already-processed GIFs
    to_process = [g for g in gifs if g["url"] not in processed_urls]
    print(f"{len(to_process)} GIFs to process with {args.workers} workers")

    if not to_process:
        print("Nothing to do!")
        return

    # Thread-safe counters and file writing
    lock = threading.Lock()
    success = 0
    failed = 0
    removed = 0
    start = time.time()

    output_mode = "a" if args.resume else "w"
    with open(args.output, output_mode) as out:
        with ThreadPoolExecutor(max_workers=args.workers) as executor:
            futures = {
                executor.submit(process_gif, client, gif): gif
                for gif in to_process
            }

            for future in as_completed(futures):
                result = future.result()
                with lock:
                    if result is REMOVED:
                        removed += 1
                    elif result:
                        out.write(json.dumps(result) + "\n")
                        out.flush()
                        success += 1
                    else:
                        failed += 1

                    done = success + failed + removed
                    if done % 10 == 0 or done == len(to_process):
                        elapsed = time.time() - start
                        rate = done / elapsed if elapsed > 0 else 0
                        msg = (f"  Progress: {done}/{len(to_process)} — "
                               f"{success} ok, {failed} failed, {removed} removed "
                               f"({rate:.1f}/sec)")
                        print(f"\r{msg}\033[K", end="", flush=True)

    print()  # finish the progress line
    elapsed = time.time() - start
    print(f"Done! {success} described, {failed} failed, {removed} removed in {elapsed:.1f}s")
    print(f"Output: {args.output}")

    # Rough cost estimate
    # ~1500 input tokens per GIF (5 frames), ~150 output tokens
    input_tokens = success * 1500
    output_tokens = success * 150
    cost = (input_tokens * 0.08 / 1_000_000) + (output_tokens * 0.30 / 1_000_000)
    print(f"Estimated cost: ${cost:.4f}")


if __name__ == "__main__":
    main()
