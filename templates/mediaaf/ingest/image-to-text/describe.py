#!/usr/bin/env python3
# /// script
# requires-python = ">=3.11"
# dependencies = ["google-genai", "Pillow", "httpx"]
# ///
"""
Media description pipeline - generates rich text descriptions for MediaAF.

Usage:
    # Local corpus directory
    uv run ingest/image-to-text/describe.py --source local --local-dir ./corpus --limit 10

    # Manifest with local paths or URLs
    uv run ingest/image-to-text/describe.py --source manifest --manifest-file ./corpus/manifest.json

    # Resume from checkpoint
    uv run ingest/image-to-text/describe.py --source local --local-dir ./corpus --resume
"""

import argparse
import io
import json
import os
import sys
import threading
import time
from abc import ABC, abstractmethod
from concurrent.futures import ThreadPoolExecutor, as_completed
from dataclasses import dataclass, field
from pathlib import Path
from typing import Iterator
from urllib.request import Request, urlopen

from PIL import Image

# Frame extraction settings
NUM_FRAMES = 5
MAX_FRAME_DIM = 512

# Default paths
DEFAULT_PROMPT_FILE = Path(__file__).parent / "prompt.txt"
DEFAULT_OUTPUT_DIR = Path(__file__).parent / "output"


# =============================================================================
# Data Classes
# =============================================================================

@dataclass
class GifItem:
    """A media to be processed."""
    id: str
    source_path: str  # URL or local path
    dataset: str
    original_url: str = ""
    original_description: str = ""
    attribution: str = ""


@dataclass
class ProcessingState:
    """Track processing progress for resumability."""
    processed_ids: set = field(default_factory=set)
    state_file: Path = None

    def load(self, path: Path):
        """Load state from file."""
        self.state_file = path
        if path.exists():
            with open(path) as f:
                data = json.load(f)
                self.processed_ids = set(data.get("processed_ids", []))

    def save(self):
        """Save state to file."""
        if self.state_file:
            with open(self.state_file, "w") as f:
                json.dump({"processed_ids": list(self.processed_ids)}, f)

    def mark_processed(self, item_id: str):
        """Mark an item as processed."""
        self.processed_ids.add(item_id)

    def is_processed(self, item_id: str) -> bool:
        """Check if an item has been processed."""
        return item_id in self.processed_ids


# =============================================================================
# Media sources
# =============================================================================

class GifSource(ABC):
    """Abstract base class for media data sources."""

    @abstractmethod
    def list_items(self, limit: int = 0) -> Iterator[GifItem]:
        """List available media items."""
        pass

    @abstractmethod
    def download(self, item: GifItem) -> bytes | None:
        """Download or read media data for an item."""
        pass


def _stable_id(value: str, prefix: str = "media") -> str:
    import hashlib
    return f"{prefix}_{hashlib.md5(value.encode()).hexdigest()[:10]}"


def _is_url(value: str) -> bool:
    return value.startswith("http://") or value.startswith("https://")


def _download_url(url: str) -> bytes | None:
    try:
        req = Request(url, headers={"User-Agent": "Mozilla/5.0"})
        with urlopen(req, timeout=30) as resp:
            return resp.read()
    except Exception as e:
        print(f"  Download error for {url[:80]}: {e}", file=sys.stderr)
        return None


class LocalSource(GifSource):
    """Load media items from a local directory."""

    EXTENSIONS = (".gif", ".png", ".jpg", ".jpeg", ".webp")

    def __init__(self, directory: Path):
        self.directory = Path(directory)
        if not self.directory.exists():
            raise FileNotFoundError(f"Directory not found: {directory}")

    def list_items(self, limit: int = 0) -> Iterator[GifItem]:
        count = 0
        for path in sorted(self.directory.rglob("*")):
            if not path.is_file() or path.suffix.lower() not in self.EXTENSIONS:
                continue
            yield GifItem(
                id=_stable_id(str(path), "local"),
                source_path=str(path),
                dataset="local",
                attribution=str(path),
            )
            count += 1
            if limit and count >= limit:
                break

    def download(self, item: GifItem) -> bytes | None:
        try:
            return Path(item.source_path).read_bytes()
        except Exception as e:
            print(f"  Local read error: {e}", file=sys.stderr)
            return None


class ManifestSource(GifSource):
    """Load media items from a JSON manifest with local paths or URLs.

    Supported item fields: id, path/source_path/local_path/url/original_url,
    dataset, description, attribution. Relative paths are resolved relative to
    the manifest file.
    """

    def __init__(self, manifest_path: Path):
        self.manifest_path = Path(manifest_path)
        if not self.manifest_path.exists():
            raise FileNotFoundError(f"Manifest not found: {manifest_path}")
        with open(self.manifest_path) as f:
            data = json.load(f)
        self.items_data = data.get("items", data if isinstance(data, list) else [])

    def list_items(self, limit: int = 0) -> Iterator[GifItem]:
        count = 0
        base = self.manifest_path.parent
        for entry in self.items_data:
            source = (
                entry.get("source_path")
                or entry.get("path")
                or entry.get("local_path")
                or entry.get("url")
                or entry.get("original_url")
            )
            if not source:
                continue
            source_path = source if _is_url(source) else str((base / source).resolve())
            yield GifItem(
                id=entry.get("id") or _stable_id(source_path, "manifest"),
                source_path=source_path,
                dataset=entry.get("dataset", "manifest"),
                original_url=entry.get("original_url", source if _is_url(source) else ""),
                original_description=entry.get("description", ""),
                attribution=entry.get("attribution", ""),
            )
            count += 1
            if limit and count >= limit:
                break

    def download(self, item: GifItem) -> bytes | None:
        if _is_url(item.source_path):
            return _download_url(item.source_path)
        try:
            return Path(item.source_path).read_bytes()
        except Exception as e:
            print(f"  Manifest read error: {e}", file=sys.stderr)
            return None


# =============================================================================
# Description API clients
# =============================================================================

class DescriptionClient(ABC):
    """Abstract base class for media-description clients."""

    def __init__(self):
        self.total_input_tokens = 0
        self.total_output_tokens = 0
        self.last_error = None

    @abstractmethod
    def generate(self, prompt: str, images: list[bytes]) -> str | None:
        """Generate text from prompt and images."""
        pass


class GoogleGenAIClient(DescriptionClient):
    """Client using google-genai SDK (current approach)."""

    def __init__(self, api_key: str = None, model: str = "gemini-2.0-flash-lite"):
        super().__init__()
        from google import genai
        self.genai = genai
        self.model = model

        # Load API key explicitly. Templates should stop and ask the user instead
        # of silently switching providers or reading unrelated local token files.
        if api_key:
            key = api_key
        elif os.environ.get("GEMINI_API_KEY"):
            key = os.environ["GEMINI_API_KEY"]
        elif os.environ.get("GOOGLE_API_KEY"):
            key = os.environ["GOOGLE_API_KEY"]
        else:
            raise ValueError(
                "No Gemini API key found. Set GEMINI_API_KEY/GOOGLE_API_KEY, "
                "or choose another DESCRIPTION_PROVIDER after asking the user."
            )

        self.client = genai.Client(api_key=key)

    def _build_contents(self, prompt: str, images: list[bytes]):
        from google.genai import types
        parts = [types.Part.from_text(text=prompt)]
        for img_data in images:
            parts.append(types.Part.from_bytes(data=img_data, mime_type="image/png"))
        return [types.Content(parts=parts)]

    def generate(self, prompt: str, images: list[bytes]) -> str | None:
        """Generate text from prompt and images."""
        try:
            response = self.client.models.generate_content(
                model=self.model,
                contents=self._build_contents(prompt, images)
            )
            if response.usage_metadata:
                self.total_input_tokens += response.usage_metadata.prompt_token_count or 0
                self.total_output_tokens += response.usage_metadata.candidates_token_count or 0
            if not response.text:
                if response.prompt_feedback and response.prompt_feedback.block_reason:
                    self.last_error = f"blocked: {response.prompt_feedback.block_reason.name}"
                elif response.candidates and response.candidates[0].finish_reason:
                    self.last_error = f"finish: {response.candidates[0].finish_reason.name}"
                else:
                    self.last_error = "empty response"
                return None
            return response.text
        except Exception as e:
            self.last_error = str(e)
            return None



class AntflyCloudInferenceClient(DescriptionClient):
    """Client for Antfly Cloud Inference's OpenAI-compatible chat endpoint."""

    def __init__(self, url: str = None, model: str = "gemini-2.5-flash", api_key: str = None):
        super().__init__()
        import httpx
        self.httpx = httpx
        self.url = (url or os.environ.get("ANTFLY_INFERENCE_URL") or "").rstrip("/")
        self.model = model
        self.api_key = (
            api_key
            or os.environ.get("ANTFLY_INFERENCE_API_KEY")
            or os.environ.get("ANTFLYDB_API_KEY")
            or os.environ.get("ANTFLY_TOKEN")
            or os.environ.get("ANTFLY_CLOUD_TOKEN")
        )
        if not self.url:
            raise ValueError(
                "ANTFLY_INFERENCE_URL is required for backend=antfly. "
                "Run `antfly cloud connection <instance> --json` and use "
                "the antfly_inference_proxy_url value."
            )
        if not self.api_key:
            raise ValueError(
                "An Antfly Cloud token/key is required for backend=antfly. "
                "Set ANTFLY_INFERENCE_API_KEY, ANTFLYDB_API_KEY, ANTFLY_TOKEN, "
                "or ANTFLY_CLOUD_TOKEN."
            )

    def generate(self, prompt: str, images: list[bytes]) -> str | None:
        """Generate text via Antfly Cloud Inference using OpenAI vision message shape."""
        import base64

        content = [{"type": "text", "text": prompt}]
        for img_data in images:
            b64 = base64.b64encode(img_data).decode()
            content.append({
                "type": "image_url",
                "image_url": {"url": f"data:image/png;base64,{b64}"}
            })

        try:
            resp = self.httpx.post(
                f"{self.url}/chat/completions",
                headers={"Authorization": f"Bearer {self.api_key}"},
                json={
                    "model": self.model,
                    "messages": [{"role": "user", "content": content}],
                    "max_tokens": 2048,
                },
                timeout=300.0,
            )
            resp.raise_for_status()
            data = resp.json()
            usage = data.get("usage", {})
            self.total_input_tokens += usage.get("prompt_tokens", 0)
            self.total_output_tokens += usage.get("completion_tokens", 0)
            return data["choices"][0]["message"]["content"]
        except Exception as e:
            self.last_error = str(e)
            return None


class OpenRouterClient(DescriptionClient):
    """Client using OpenRouter (OpenAI-compatible vision API)."""

    def __init__(self, model: str = "google/gemma-4-4b-it", api_key: str = None):
        super().__init__()
        import httpx
        self.httpx = httpx
        self.model = model
        if api_key:
            self.api_key = api_key
        elif os.environ.get("OPENROUTER_API_KEY"):
            self.api_key = os.environ["OPENROUTER_API_KEY"]
        else:
            raise ValueError(
                "No OpenRouter API key found. Set OPENROUTER_API_KEY, "
                "or choose another DESCRIPTION_PROVIDER after asking the user."
            )

    def generate(self, prompt: str, images: list[bytes]) -> str | None:
        """Generate text from prompt and images using OpenRouter."""
        import base64
        import time

        content = [{"type": "text", "text": prompt}]
        for img_data in images:
            b64 = base64.b64encode(img_data).decode()
            content.append({
                "type": "image_url",
                "image_url": {"url": f"data:image/png;base64,{b64}"}
            })

        for attempt in range(2):
            try:
                resp = self.httpx.post(
                    "https://openrouter.ai/api/v1/chat/completions",
                    headers={"Authorization": f"Bearer {self.api_key}"},
                    json={
                        "model": self.model,
                        "messages": [{"role": "user", "content": content}],
                        "max_tokens": 2048,
                    },
                    timeout=120.0,
                )
                if resp.status_code in (429, 500, 502, 503):
                    wait = 10 * (attempt + 1)
                    self.last_error = f"OpenRouter {resp.status_code}, retrying ({attempt + 1}/2)"
                    time.sleep(wait)
                    continue
                resp.raise_for_status()
                data = resp.json()
                if "choices" not in data:
                    error = data.get("error", data)
                    self.last_error = f"OpenRouter API error: {error}"
                    return None
                usage = data.get("usage", {})
                self.total_input_tokens += usage.get("prompt_tokens", 0)
                self.total_output_tokens += usage.get("completion_tokens", 0)
                return data["choices"][0]["message"]["content"]
            except Exception as e:
                self.last_error = str(e)
                if attempt < 1:
                    time.sleep(10 * (attempt + 1))
                    continue
                return None
        self.last_error = "OpenRouter: failed after 2 attempts"
        return None


# =============================================================================
# Core Processing Logic
# =============================================================================

def extract_frames(gif_data: bytes, num_frames: int = NUM_FRAMES, max_dim: int = MAX_FRAME_DIM) -> list[bytes]:
    """Extract evenly-spaced frames from a media as PNG bytes."""
    img = Image.open(io.BytesIO(gif_data))
    n_frames = getattr(img, "n_frames", 1)

    # Pick frame indices: evenly spaced including first and last
    if num_frames == 1:
        # Single frame: pick middle frame for best representation
        indices = [n_frames // 2]
    elif n_frames <= num_frames:
        indices = list(range(n_frames))
    else:
        indices = [round(i * (n_frames - 1) / (num_frames - 1)) for i in range(num_frames)]

    frames = []
    for idx in indices:
        img.seek(idx)
        frame = img.convert("RGB")

        # Resize if too large
        w, h = frame.size
        if max(w, h) > max_dim:
            scale = max_dim / max(w, h)
            frame = frame.resize((round(w * scale), round(h * scale)), Image.BILINEAR)

        buf = io.BytesIO()
        frame.save(buf, format="PNG")
        frames.append(buf.getvalue())

    return frames


def clean_json_response(text: str) -> str:
    """Strip markdown fences and trailing garbage from JSON responses."""
    text = text.strip()

    # Remove unicode spacing chars that Gemma sometimes adds (U+2581 lower one-eighth block)
    text = text.replace("\u2581", "")

    # Handle markdown code fences (Gemma sometimes outputs ```json...`````` with extra backticks)
    if text.startswith("```"):
        lines = text.split("\n")
        # Remove first line (```json or ```)
        lines = lines[1:]
        # Remove trailing lines that are empty or just backticks
        while lines and (not lines[-1].strip() or lines[-1].strip().startswith("`")):
            lines = lines[:-1]
        text = "\n".join(lines)

    # Also handle case where backticks appear after closing brace on same line or subsequent lines
    # Find the last } and truncate there
    if "{" in text:
        brace_count = 0
        last_close = -1
        for i, c in enumerate(text):
            if c == "{":
                brace_count += 1
            elif c == "}":
                brace_count -= 1
                if brace_count == 0:
                    last_close = i
        if last_close > 0:
            text = text[:last_close + 1]

    return text.strip()


def process_item(
    client: DescriptionClient,
    source: GifSource,
    item: GifItem,
    prompt: str,
    num_frames: int = NUM_FRAMES,
    retries: int = 1
) -> tuple[dict | None, str | None]:
    """Process a single media item. Returns (result_dict, failure_reason) tuple."""
    # Download media
    gif_data = source.download(item)
    if gif_data is None:
        return None, "download"

    # Extract frames
    try:
        frames = extract_frames(gif_data, num_frames=num_frames)
    except Exception as e:
        return None, f"frames: {e}"

    # Generate description
    for attempt in range(1 + retries):
        response = client.generate(prompt, frames)
        if response is None:
            continue

        try:
            text = clean_json_response(response)
            data = json.loads(text)
            # Add metadata
            data["id"] = item.id
            data["dataset"] = item.dataset
            data["source_path"] = item.source_path
            if item.original_url:
                data["original_url"] = item.original_url
            if item.original_description:
                data["original_description"] = item.original_description
            if item.attribution:
                data["attribution"] = item.attribution
            return data, None
        except json.JSONDecodeError as e:
            if attempt >= retries:
                return None, f"json: {e}"

    return None, f"api: {client.last_error or 'no response'}"


# =============================================================================
# Main Entry Point
# =============================================================================

def main():
    parser = argparse.ArgumentParser(description="MediaAF description pipeline")
    parser.add_argument("--source", choices=["local", "manifest"], default="local",
                        help="Data source type")
    parser.add_argument("--local-dir", type=Path, help="Local directory (for local source)")
    parser.add_argument("--manifest-file", type=Path,
                        help="Path to manifest JSON file (for manifest source)")
    parser.add_argument("--limit", type=int, default=100,
                        help="Limit items to process (0=all)")
    parser.add_argument("--output", type=Path, default=DEFAULT_OUTPUT_DIR / "descriptions.jsonl",
                        help="Output JSONL file")
    parser.add_argument("--prompt", type=Path, default=DEFAULT_PROMPT_FILE,
                        help="Prompt file")
    parser.add_argument("--resume", action="store_true",
                        help="Resume from checkpoint")
    parser.add_argument("--only-unprocessed-by",
                        help="Only process items that failed/were skipped by this model (reads its state file)")
    parser.add_argument("--workers", type=int, default=20,
                        help="Number of concurrent workers")
    parser.add_argument("--frames", type=int, default=NUM_FRAMES,
                        help=f"Number of frames to extract from each media (default: {NUM_FRAMES})")
    parser.add_argument("--model", default="gemini-2.5-flash",
                        help="Model name for the selected backend")
    parser.add_argument("--backend", choices=["antfly", "genai", "openrouter"], default="antfly",
                        help="API backend. Prefer antfly, or choose one direct hosted provider explicitly.")
    parser.add_argument("--antfly-inference-url", default=os.environ.get("ANTFLY_INFERENCE_URL", ""),
                        help="Antfly Cloud inference proxy URL, usually antfly_inference_proxy_url from `antfly cloud connection <instance> --json`")
    args = parser.parse_args()

    # Ensure output directory exists
    args.output.parent.mkdir(parents=True, exist_ok=True)

    # Load prompt
    if not args.prompt.exists():
        print(f"Error: Prompt file not found: {args.prompt}", file=sys.stderr)
        sys.exit(1)
    with open(args.prompt) as f:
        prompt = f.read().strip()
    print(f"Loaded prompt from {args.prompt} ({len(prompt)} chars)")

    # Initialize source
    if args.source == "local":
        if not args.local_dir:
            print("Error: --local-dir required for local source", file=sys.stderr)
            sys.exit(1)
        source = LocalSource(args.local_dir)
        print(f"Using local source: {args.local_dir}")
    elif args.source == "manifest":
        if not args.manifest_file:
            print("Error: --manifest-file required for manifest source", file=sys.stderr)
            sys.exit(1)
        source = ManifestSource(args.manifest_file)
        print(f"Using manifest source: {args.manifest_file} ({len(source.items_data)} items)")

    # Initialize client based on backend. Do not silently fall back between
    # providers; missing access/credentials should be fixed by the user.
    try:
        if args.backend == "antfly":
            client = AntflyCloudInferenceClient(url=args.antfly_inference_url, model=args.model)
            print(f"Using Antfly Cloud Inference: {args.antfly_inference_url}, model: {args.model}")
        elif args.backend == "openrouter":
            client = OpenRouterClient(model=args.model)
            print(f"Using OpenRouter, model: {args.model}")
        else:
            client = GoogleGenAIClient(model=args.model)
            print(f"Using Google GenAI, model: {args.model}")
    except Exception as e:
        print(f"Error configuring inference backend '{args.backend}': {e}", file=sys.stderr)
        print("Do not silently fall back to another provider; ask the user which provider/model to use.", file=sys.stderr)
        sys.exit(1)

    # Load processing state
    state = ProcessingState()
    state_file = args.output.with_suffix(".state.json")
    if args.resume:
        state.load(state_file)
        print(f"Resuming: {len(state.processed_ids)} already processed")

    # List items to process
    print(f"Listing items (limit={args.limit})...")
    items = list(source.list_items(args.limit))
    print(f"Found {len(items)} items")

    # Filter to only items unprocessed by another model
    if args.only_unprocessed_by:
        other_state_file = args.output.parent / f"descriptions-{args.only_unprocessed_by}.state.json"
        if not other_state_file.exists():
            print(f"Error: state file not found: {other_state_file}", file=sys.stderr)
            sys.exit(1)
        other_state = ProcessingState()
        other_state.load(other_state_file)
        all_ids = {item.id for item in items}
        unprocessed_ids = all_ids - other_state.processed_ids
        items = [item for item in items if item.id in unprocessed_ids]
        print(f"Filtered to {len(items)} items unprocessed by {args.only_unprocessed_by}")

    # Filter already processed
    to_process = [item for item in items if not state.is_processed(item.id)]
    print(f"{len(to_process)} items to process with {args.workers} workers")

    if not to_process:
        print("Nothing to do!")
        return

    success = 0
    failed = 0
    start = time.time()
    output_mode = "a" if args.resume else "w"

    # Standard concurrent mode
    lock = threading.Lock()
    with open(args.output, output_mode) as out:
        with ThreadPoolExecutor(max_workers=args.workers) as executor:
            futures = {
                executor.submit(process_item, client, source, item, prompt, args.frames): item
                for item in to_process
            }

            last_failed_shown = 0
            fail_reasons: dict[str, int] = {}
            for future in as_completed(futures):
                item = futures[future]
                result, reason = future.result()

                with lock:
                    if result:
                        out.write(json.dumps(result) + "\n")
                        out.flush()
                        success += 1
                        state.mark_processed(item.id)
                    else:
                        failed += 1
                        # Bucket by reason, truncated for grouping
                        bucket = (reason or "unknown")[:40]
                        fail_reasons[bucket] = fail_reasons.get(bucket, 0) + 1

                    done = success + failed
                    if done % 10 == 0 or done == len(to_process):
                        elapsed = time.time() - start
                        rate = done / elapsed if elapsed > 0 else 0
                        if done % 100 == 0:
                            state.save()
                        msg = (f"  Progress: {done}/{len(to_process)} — "
                               f"{success} ok, {failed} failed ({rate:.1f}/sec)")
                        if client.last_error and failed > last_failed_shown:
                            err_short = client.last_error[:60]
                            msg += f"  [err: {err_short}]"
                            last_failed_shown = failed
                        print(f"\r{msg}\033[K", end="", flush=True)
    print()

    # Final state save
    state.save()

    elapsed = time.time() - start
    print(f"Done! {success} described, {failed} failed in {elapsed:.1f}s")
    if fail_reasons:
        breakdown = ", ".join(f"{k}: {v}" for k, v in sorted(fail_reasons.items(), key=lambda x: -x[1]))
        print(f"Failure breakdown: {breakdown}")
    print(f"Output: {args.output}")
    print(f"State: {state_file}")

    # Token usage
    input_tokens = client.total_input_tokens
    output_tokens = client.total_output_tokens
    token_source = "actual"
    if input_tokens == 0 and success > 0:
        # Fallback estimate if API did not return usage.
        input_tokens = success * 1500
        output_tokens = success * 200
        token_source = "estimated"

    print(f"Tokens: {input_tokens:,} in / {output_tokens:,} out ({token_source})")

    # Write summary alongside JSONL.
    summary_path = args.output.with_suffix(".summary.json")
    summary = {
        "model": args.model,
        "backend": args.backend,
        "items": success,
        "failed": failed,
        "input_tokens": input_tokens,
        "output_tokens": output_tokens,
        "token_source": token_source,
        "elapsed_s": round(elapsed, 1),
    }
    with open(summary_path, "w") as f:
        json.dump(summary, f, indent=2)
        f.write("\n")


if __name__ == "__main__":
    main()
