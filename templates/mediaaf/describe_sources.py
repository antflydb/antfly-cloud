#!/usr/bin/env python3
# /// script
# requires-python = ">=3.11"
# dependencies = ["google-genai", "Pillow", "python-dotenv"]
# ///
"""
Generate rich text descriptions of GIFs/videos from source manifests using Gemini.

Iterates all sources/*/manifest.json, finds items that are downloaded but not yet
described, extracts frames, sends to Gemini, and writes per-source descriptions.jsonl.

This is the source-agnostic counterpart of describe_gifs.py (which is TGIF-specific).

Usage:
    uv run describe_sources.py                          # All sources
    uv run describe_sources.py --source tgif             # Single source
    uv run describe_sources.py --limit 10 --workers 5   # Limited run
    uv run describe_sources.py --force                  # Re-describe all
"""

import argparse
import io
import json
import os
import subprocess
import sys
import tempfile
import threading
import time
from concurrent.futures import ThreadPoolExecutor, as_completed
from pathlib import Path

from dotenv import load_dotenv
from PIL import Image
from google import genai
from google.genai import types

load_dotenv()

# Config
SOURCES_DIR = Path(__file__).parent / "sources"
API_KEY_ENV_VAR = "GEMINI_API_KEY"
MODEL_NAME = "gemini-2.0-flash-lite"

# Frame extraction settings
NUM_FRAMES = 5
MAX_FRAME_DIM = 512

# Same prompt as describe_gifs.py
DESCRIPTION_PROMPT = """You are analyzing multiple frames extracted from an animated GIF. The frames are shown in chronological order.

Analyze the FULL sequence of action across all frames and return a JSON object:
- "literal": Factual description of the complete action/sequence (1-3 sentences, describe what happens from start to finish)
- "source": Your best guess at where this is from — movie title, TV show, meme name, video game, news event, YouTube/TikTok trend, etc. Be specific (e.g., "Spy Kids (2001)" not just "movie"). Use "unknown" only if you genuinely cannot identify it.
- "mood": Emotional tone or vibe (e.g., "funny", "wholesome", "chaotic", "satisfying")
- "action": Key actions/verbs (e.g., "dancing", "falling", "celebrating")
- "context": When someone might use this GIF in conversation (e.g., "reaction to good news")
- "tags": Array of 5-10 searchable keywords (include character names, show titles, meme names if recognized)

Respond with ONLY the JSON object, no markdown or extra text."""


def load_api_key() -> str:
    key = os.environ.get(API_KEY_ENV_VAR)
    if not key:
        print(f"Error: {API_KEY_ENV_VAR} environment variable not set", file=sys.stderr)
        sys.exit(1)
    return key


def find_sources(source_filter: str | None = None) -> list[Path]:
    """Find all source directories with a manifest.json."""
    sources = []
    if not SOURCES_DIR.exists():
        return sources
    for d in sorted(SOURCES_DIR.iterdir()):
        if not d.is_dir() or d.name.startswith("_"):
            continue
        if source_filter and d.name != source_filter:
            continue
        if (d / "manifest.json").exists():
            sources.append(d)
    return sources


def load_manifest(source_dir: Path) -> dict:
    with open(source_dir / "manifest.json") as f:
        return json.load(f)


def save_manifest(source_dir: Path, manifest: dict) -> None:
    tmp = (source_dir / "manifest.json").with_suffix(".tmp")
    with open(tmp, "w") as f:
        json.dump(manifest, f, indent=2)
    tmp.rename(source_dir / "manifest.json")


def items_needing_description(manifest: dict, force: bool = False) -> list[dict]:
    """Return items that are downloaded but not yet described."""
    return [
        item for item in manifest["items"]
        if item.get("downloaded") and (force or not item.get("described"))
    ]


def extract_frames_gif(file_path: Path, num_frames: int = NUM_FRAMES,
                       max_dim: int = MAX_FRAME_DIM) -> list[bytes]:
    """Extract evenly-spaced frames from a GIF as PNG bytes."""
    img = Image.open(file_path)
    n_frames = getattr(img, "n_frames", 1)

    if n_frames <= num_frames:
        indices = list(range(n_frames))
    else:
        indices = [round(i * (n_frames - 1) / (num_frames - 1)) for i in range(num_frames)]

    frames = []
    for idx in indices:
        img.seek(idx)
        frame = img.convert("RGBA")

        w, h = frame.size
        if max(w, h) > max_dim:
            scale = max_dim / max(w, h)
            frame = frame.resize((round(w * scale), round(h * scale)), Image.LANCZOS)

        buf = io.BytesIO()
        frame.save(buf, format="PNG")
        frames.append(buf.getvalue())

    return frames


def extract_frames_mp4(file_path: Path, num_frames: int = NUM_FRAMES,
                       max_dim: int = MAX_FRAME_DIM) -> list[bytes]:
    """Extract evenly-spaced frames from an MP4/WebM using ffmpeg."""
    # Get video duration and frame count
    probe = subprocess.run(
        ["ffprobe", "-v", "quiet", "-print_format", "json",
         "-show_streams", str(file_path)],
        capture_output=True, text=True
    )
    if probe.returncode != 0:
        raise RuntimeError(f"ffprobe failed: {probe.stderr}")

    streams = json.loads(probe.stdout)
    video_stream = next(
        (s for s in streams.get("streams", []) if s["codec_type"] == "video"),
        None
    )
    if not video_stream:
        raise RuntimeError("No video stream found")

    # Get duration (try multiple fields)
    duration = float(video_stream.get("duration", 0))
    if duration == 0:
        # Try nb_frames / fps
        nb = int(video_stream.get("nb_frames", 0))
        r_frame_rate = video_stream.get("r_frame_rate", "30/1")
        num, den = map(int, r_frame_rate.split("/"))
        if nb > 0 and num > 0:
            duration = nb * den / num

    if duration <= 0:
        # Fallback: just grab the first frame
        duration = 1.0

    # Calculate timestamps for evenly-spaced frames
    if num_frames == 1:
        timestamps = [0.0]
    else:
        timestamps = [i * duration / (num_frames - 1) for i in range(num_frames)]
        # Clamp last timestamp slightly before end
        timestamps[-1] = min(timestamps[-1], max(0, duration - 0.01))

    frames = []
    with tempfile.TemporaryDirectory() as tmpdir:
        for i, ts in enumerate(timestamps):
            out_path = os.path.join(tmpdir, f"frame_{i}.png")
            cmd = [
                "ffmpeg", "-v", "quiet",
                "-ss", f"{ts:.3f}",
                "-i", str(file_path),
                "-frames:v", "1",
                "-vf", f"scale='min({max_dim},iw)':'min({max_dim},ih)':force_original_aspect_ratio=decrease",
                out_path
            ]
            subprocess.run(cmd, capture_output=True)
            if os.path.exists(out_path):
                frames.append(Path(out_path).read_bytes())

    if not frames:
        raise RuntimeError("ffmpeg extracted no frames")
    return frames


def extract_frames(file_path: Path, fmt: str) -> list[bytes]:
    """Dispatch to the right frame extractor based on format."""
    if fmt in ("mp4", "webm"):
        return extract_frames_mp4(file_path)
    else:
        return extract_frames_gif(file_path)


def describe_item(client, source_dir: Path, item: dict) -> dict | None:
    """Generate description for a single item."""
    local_path = source_dir / item["local_file"]
    if not local_path.exists():
        print(f"  File not found: {local_path}", file=sys.stderr)
        return None

    try:
        frames = extract_frames(local_path, item.get("format", "gif"))
    except Exception as e:
        print(f"  Frame extraction error for {item['id']}: {e}", file=sys.stderr)
        return None

    try:
        parts = [types.Part.from_text(text=DESCRIPTION_PROMPT)]
        for frame_png in frames:
            parts.append(types.Part.from_bytes(data=frame_png, mime_type="image/png"))

        response = client.models.generate_content(
            model=MODEL_NAME,
            contents=[types.Content(parts=parts)]
        )

        text = response.text.strip()
        if text.startswith("```"):
            lines = text.split("\n")
            text = "\n".join(lines[1:-1])

        desc = json.loads(text)

        return {
            "id": item["id"],
            "url": item["original_url"],
            "attribution": item.get("page_url", ""),
            "original_description": item.get("title", ""),
            **desc,
        }
    except json.JSONDecodeError as e:
        print(f"  JSON parse error for {item['id']}: {e}", file=sys.stderr)
        return None
    except Exception as e:
        print(f"  API error for {item['id']}: {e}", file=sys.stderr)
        return None


def process_source(client, source_dir: Path, manifest: dict,
                   workers: int, limit: int, force: bool) -> int:
    """Process all undescribed items for a source. Returns count processed."""
    to_process = items_needing_description(manifest, force=force)
    if limit:
        to_process = to_process[:limit]

    if not to_process:
        return 0

    source_name = manifest["source"]
    descriptions_path = source_dir / "descriptions.jsonl"

    # Load existing descriptions for resume support
    existing_ids = set()
    if descriptions_path.exists() and not force:
        with open(descriptions_path) as f:
            for line in f:
                try:
                    existing_ids.add(json.loads(line)["id"])
                except (json.JSONDecodeError, KeyError):
                    pass
        to_process = [i for i in to_process if i["id"] not in existing_ids]

    if not to_process:
        print(f"  All items already described")
        return 0

    print(f"  {len(to_process)} items to describe with {workers} workers")

    lock = threading.Lock()
    success = 0
    failed = 0
    start = time.time()

    output_mode = "a" if existing_ids else "w"
    if force:
        output_mode = "w"

    with open(descriptions_path, output_mode) as out:
        with ThreadPoolExecutor(max_workers=workers) as executor:
            futures = {
                executor.submit(describe_item, client, source_dir, item): item
                for item in to_process
            }

            for future in as_completed(futures):
                item = futures[future]
                result = future.result()
                with lock:
                    if result:
                        out.write(json.dumps(result) + "\n")
                        out.flush()
                        item["described"] = True
                        success += 1
                    else:
                        failed += 1

                    done = success + failed
                    if done % 10 == 0 or done == len(to_process):
                        elapsed = time.time() - start
                        rate = done / elapsed if elapsed > 0 else 0
                        print(f"  [{source_name}] {success} ok, {failed} fail, "
                              f"{done}/{len(to_process)} ({rate:.1f}/sec)")

    # Save manifest with updated described status
    save_manifest(source_dir, manifest)

    return success


def main():
    parser = argparse.ArgumentParser(description="Describe GIFs from source manifests")
    parser.add_argument("--source", help="Process only this source (directory name)")
    parser.add_argument("--limit", type=int, default=0, help="Limit items per source (0=all)")
    parser.add_argument("--workers", type=int, default=10, help="Concurrent workers (default: 10)")
    parser.add_argument("--force", action="store_true", help="Re-describe all items")
    args = parser.parse_args()

    api_key = load_api_key()
    client = genai.Client(api_key=api_key)

    sources = find_sources(args.source)
    if not sources:
        print("No sources found. Run a scraper first (e.g., uv run sources/tgif/scrape.py)")
        return

    print(f"Found {len(sources)} source(s): {', '.join(s.name for s in sources)}")

    total_success = 0
    for source_dir in sources:
        manifest = load_manifest(source_dir)
        source_name = manifest["source"]
        total = len(manifest["items"])
        downloaded = sum(1 for i in manifest["items"] if i.get("downloaded"))
        described = sum(1 for i in manifest["items"] if i.get("described"))

        print(f"\n=== {source_name} === ({total} items, {downloaded} downloaded, {described} described)")

        count = process_source(client, source_dir, manifest, args.workers, args.limit, args.force)
        total_success += count

    print(f"\nDone! Described {total_success} items total across {len(sources)} source(s)")


if __name__ == "__main__":
    main()
