#!/usr/bin/env python3
# /// script
# requires-python = ">=3.11"
# dependencies = ["google-genai", "google-cloud-aiplatform", "Pillow", "boto3", "httpx", "pyyaml"]
# ///
"""
Media description pipeline - generates rich text descriptions using Gemini.

Usage:
    # Local media items (from TGIF TSV)
    uv run ingest/image-to-text/describe.py --source tgif --limit 100

    # From R2 bucket (requires R2 credentials)
    uv run ingest/image-to-text/describe.py --source r2 --limit 100

    # Resume from checkpoint
    uv run ingest/image-to-text/describe.py --source r2 --resume

    # Use custom prompt
    uv run ingest/image-to-text/describe.py --prompt my_prompt.txt --limit 10
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
from urllib.request import urlopen, Request

from PIL import Image

# Frame extraction settings
NUM_FRAMES = 5
MAX_FRAME_DIM = 512

# Default paths
DEFAULT_PROMPT_FILE = Path(__file__).parent / "prompt.txt"
DEFAULT_OUTPUT_DIR = Path(__file__).parent / "output"
DEFAULT_TSV = Path(__file__).parent.parent.parent / "../datasets/TGIF-Release/data/tgif-v1.0.tsv"


# =============================================================================
# Data Classes
# =============================================================================

@dataclass
class GifItem:
    """A media to be processed."""
    id: str
    source_path: str  # R2 path, URL, or local path
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
# media Sources - Abstract interface for different data sources
# =============================================================================

class GifSource(ABC):
    """Abstract base class for media data sources."""

    @abstractmethod
    def list_items(self, limit: int = 0) -> Iterator[GifItem]:
        """List available media items."""
        pass

    @abstractmethod
    def download(self, item: GifItem) -> bytes | None:
        """Download media data for an item."""
        pass


class TGIFSource(GifSource):
    """Load media items from TGIF dataset TSV file."""

    def __init__(self, tsv_path: Path):
        self.tsv_path = Path(tsv_path)
        if not self.tsv_path.exists():
            raise FileNotFoundError(f"TGIF TSV not found: {tsv_path}")

    def _fix_tumblr_url(self, url: str) -> str:
        """Update old Tumblr CDN URLs to new domain."""
        for old in ["38.media", "33.media", "31.media"]:
            url = url.replace(f"{old}.tumblr.com", "64.media.tumblr.com")
        return url

    def list_items(self, limit: int = 0) -> Iterator[GifItem]:
        """List media items from TSV file."""
        import hashlib
        count = 0
        with open(self.tsv_path) as f:
            for line in f:
                parts = line.strip().split("\t", 1)
                if len(parts) != 2:
                    continue
                url, desc = parts
                url = self._fix_tumblr_url(url)
                # Generate stable ID from URL
                url_hash = hashlib.md5(url.encode()).hexdigest()[:8]
                yield GifItem(
                    id=f"tgif_{url_hash}",
                    source_path=url,
                    dataset="tgif",
                    original_url=url,
                    original_description=desc,
                    attribution="TGIF dataset",
                )
                count += 1
                if limit and count >= limit:
                    break

    def download(self, item: GifItem) -> bytes | None:
        """Download media from URL."""
        try:
            req = Request(item.source_path, headers={"User-Agent": "Mozilla/5.0"})
            with urlopen(req, timeout=30) as resp:
                # Detect Tumblr removal redirects
                if "assets.tumblr.com/images/media_violation/" in resp.url:
                    return None
                return resp.read()
        except Exception as e:
            print(f"  Download error: {e}", file=sys.stderr)
            return None


class R2Source(GifSource):
    """Load media items from Cloudflare R2 bucket."""

    def __init__(self, bucket: str, prefix: str = "", endpoint_url: str = None):
        import boto3
        self.bucket = bucket
        self.prefix = prefix

        # R2 uses S3-compatible API
        self.s3 = boto3.client(
            "s3",
            endpoint_url=endpoint_url or os.environ.get("R2_ENDPOINT_URL"),
            aws_access_key_id=os.environ.get("R2_ACCESS_KEY_ID"),
            aws_secret_access_key=os.environ.get("R2_SECRET_ACCESS_KEY"),
        )

    def list_items(self, limit: int = 0) -> Iterator[GifItem]:
        """List media items in R2 bucket."""
        import hashlib
        paginator = self.s3.get_paginator("list_objects_v2")
        count = 0

        for page in paginator.paginate(Bucket=self.bucket, Prefix=self.prefix):
            for obj in page.get("Contents", []):
                key = obj["Key"]
                if not key.lower().endswith((".gif", ".mp4", ".webm")):
                    continue

                # Extract dataset from path (e.g., "tgif/abc.gif" -> "tgif")
                parts = key.split("/")
                dataset = parts[0] if len(parts) > 1 else "unknown"

                # Generate stable ID
                key_hash = hashlib.md5(key.encode()).hexdigest()[:8]
                yield GifItem(
                    id=f"{dataset}_{key_hash}",
                    source_path=key,
                    dataset=dataset,
                    attribution=f"R2: {self.bucket}/{key}",
                )
                count += 1
                if limit and count >= limit:
                    return

    def download(self, item: GifItem) -> bytes | None:
        """Download media from R2."""
        try:
            response = self.s3.get_object(Bucket=self.bucket, Key=item.source_path)
            return response["Body"].read()
        except Exception as e:
            print(f"  R2 download error: {e}", file=sys.stderr)
            return None


class LocalSource(GifSource):
    """Load media items from local directory."""

    def __init__(self, directory: Path):
        self.directory = Path(directory)
        if not self.directory.exists():
            raise FileNotFoundError(f"Directory not found: {directory}")

    def list_items(self, limit: int = 0) -> Iterator[GifItem]:
        """List media items in directory."""
        import hashlib
        count = 0
        for path in self.directory.rglob("*.gif"):
            path_hash = hashlib.md5(str(path).encode()).hexdigest()[:8]
            yield GifItem(
                id=f"local_{path_hash}",
                source_path=str(path),
                dataset="local",
                attribution=str(path),
            )
            count += 1
            if limit and count >= limit:
                break

    def download(self, item: GifItem) -> bytes | None:
        """Read media from local file."""
        try:
            with open(item.source_path, "rb") as f:
                return f.read()
        except Exception as e:
            print(f"  Local read error: {e}", file=sys.stderr)
            return None


class ManifestSource(GifSource):
    """Load a fixed list of media items from a JSON manifest, downloading from R2."""

    def __init__(self, manifest_path: Path, bucket: str, endpoint_url: str = None):
        import boto3
        self.manifest_path = Path(manifest_path)
        if not self.manifest_path.exists():
            raise FileNotFoundError(f"Manifest not found: {manifest_path}")
        self.bucket = bucket

        self.s3 = boto3.client(
            "s3",
            endpoint_url=endpoint_url or os.environ.get("R2_ENDPOINT_URL"),
            aws_access_key_id=os.environ.get("R2_ACCESS_KEY_ID"),
            aws_secret_access_key=os.environ.get("R2_SECRET_ACCESS_KEY"),
        )

        with open(self.manifest_path) as f:
            self.items_data = json.load(f)

    def list_items(self, limit: int = 0) -> Iterator[GifItem]:
        """Yield GifItems from manifest."""
        count = 0
        for entry in self.items_data:
            yield GifItem(
                id=entry["id"],
                source_path=entry["source_path"],
                dataset=entry.get("dataset", "unknown"),
                attribution=entry.get("attribution", ""),
            )
            count += 1
            if limit and count >= limit:
                break

    def download(self, item: GifItem) -> bytes | None:
        """Download from R2 by key."""
        try:
            response = self.s3.get_object(Bucket=self.bucket, Key=item.source_path)
            return response["Body"].read()
        except Exception as e:
            print(f"  R2 download error: {e}", file=sys.stderr)
            return None


# =============================================================================
# Gemini API Client - Abstracted for future Vertex AI support
# =============================================================================

class GeminiClient(ABC):
    """Abstract base class for Gemini API clients."""

    def __init__(self):
        self.total_input_tokens = 0
        self.total_output_tokens = 0
        self.last_error = None

    @abstractmethod
    def generate(self, prompt: str, images: list[bytes]) -> str | None:
        """Generate text from prompt and images."""
        pass


class GoogleGenAIClient(GeminiClient):
    """Client using google-genai SDK (current approach)."""

    def __init__(self, api_key: str = None, model: str = "gemini-2.0-flash-lite"):
        super().__init__()
        from google import genai
        self.genai = genai
        self.model = model

        # Load API key
        if api_key:
            key = api_key
        elif os.environ.get("GOOGLE_API_KEY"):
            key = os.environ["GOOGLE_API_KEY"]
        else:
            key_path = Path.home() / ".tokens/gemini_api_key"
            if key_path.exists():
                with open(key_path) as f:
                    key = f.read().strip().split()[0]
            else:
                raise ValueError("No Gemini API key found. Set GOOGLE_API_KEY or create ~/.tokens/gemini_api_key")

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

    def generate_batch(self, requests: list[tuple[str, list[bytes]]]) -> list[str | None]:
        """Submit requests via the Batch API (50% cheaper). Returns list of responses."""
        inline_requests = []
        for prompt, images in requests:
            contents = self._build_contents(prompt, images)
            inline_requests.append({"contents": [{"parts": c.parts} for c in contents]})

        print(f"  Submitting batch job ({len(inline_requests)} requests)...")
        batch_job = self.client.batches.create(
            model=self.model,
            src=inline_requests,
            config={"display_name": f"describe-{len(inline_requests)}items"},
        )
        print(f"  Batch job: {batch_job.name}")

        completed_states = {
            "JOB_STATE_SUCCEEDED", "JOB_STATE_FAILED",
            "JOB_STATE_CANCELLED", "JOB_STATE_EXPIRED",
        }

        while batch_job.state.name not in completed_states:
            print(f"\r  Batch status: {batch_job.state.name}\033[K", end="", flush=True)
            time.sleep(15)
            batch_job = self.client.batches.get(name=batch_job.name)

        print(f"\r  Batch status: {batch_job.state.name}\033[K")

        if batch_job.state.name != "JOB_STATE_SUCCEEDED":
            print(f"  Batch job failed: {batch_job.state.name}", file=sys.stderr)
            return [None] * len(requests)

        results = []
        for resp in batch_job.dest.inlined_responses:
            if resp.response and resp.response.text:
                results.append(resp.response.text)
            else:
                results.append(None)
        return results


class VertexAIClient(GeminiClient):
    """Client using Vertex AI SDK (for GCP deployment)."""

    def __init__(self, project: str = None, location: str = "us-central1", model: str = "gemini-2.0-flash-001"):
        import vertexai
        from vertexai.generative_models import GenerativeModel

        self.project = project or os.environ.get("GOOGLE_CLOUD_PROJECT", "honeycomb-488503")
        self.location = location
        self.model_name = model

        vertexai.init(project=self.project, location=self.location)
        self.model = GenerativeModel(self.model_name)

    def generate(self, prompt: str, images: list[bytes]) -> str | None:
        """Generate text from prompt and images."""
        from vertexai.generative_models import Part, Image as VertexImage

        parts = [Part.from_text(prompt)]
        for img_data in images:
            parts.append(Part.from_image(VertexImage.from_bytes(img_data)))

        try:
            response = self.model.generate_content(parts)
            return response.text
        except Exception as e:
            print(f"  Vertex AI error: {e}", file=sys.stderr)
            return None


class TermiteClient(GeminiClient):
    """Client using local Termite for Gemma 3 inference."""

    def __init__(self, url: str = "http://localhost:11433", model: str = "onnxruntime/Gemma-3-ONNX"):
        import httpx
        self.httpx = httpx
        self.url = url
        self.model = model

    def generate(self, prompt: str, images: list[bytes]) -> str | None:
        """Generate text from prompt and images using Termite's OpenAI-compatible API."""
        import base64

        # Build multimodal message content (OpenAI vision format)
        content = [{"type": "text", "text": prompt}]
        for img_data in images:
            b64 = base64.b64encode(img_data).decode()
            content.append({
                "type": "image_url",
                "image_url": {"url": f"data:image/png;base64,{b64}"}
            })

        try:
            resp = self.httpx.post(
                f"{self.url}/api/generate",
                json={
                    "model": self.model,
                    "messages": [{"role": "user", "content": content}],
                    "max_tokens": 2048,
                },
                timeout=1800.0  # 30 min timeout for multi-frame CPU vision inference
            )
            resp.raise_for_status()
            return resp.json()["choices"][0]["message"]["content"]
        except Exception as e:
            print(f"  Termite error: {e}", file=sys.stderr)
            return None


class OllamaClient(GeminiClient):
    """Client using local Ollama for Gemma 3 inference."""

    def __init__(self, url: str = "http://localhost:11434", model: str = "gemma3:4b-it-qat"):
        import httpx
        self.httpx = httpx
        self.url = url
        self.model = model

    def generate(self, prompt: str, images: list[bytes]) -> str | None:
        """Generate text from prompt and images using Ollama's API."""
        import base64

        # Build message with images (Ollama format)
        message = {
            "role": "user",
            "content": prompt,
            "images": [base64.b64encode(img).decode() for img in images]
        }

        try:
            resp = self.httpx.post(
                f"{self.url}/api/chat",
                json={
                    "model": self.model,
                    "messages": [message],
                    "stream": False,
                },
                timeout=300.0  # Longer timeout for CPU inference
            )
            resp.raise_for_status()
            return resp.json()["message"]["content"]
        except Exception as e:
            print(f"  Ollama error: {e}", file=sys.stderr)
            return None


class OpenRouterClient(GeminiClient):
    """Client using OpenRouter (OpenAI-compatible vision API)."""

    def __init__(self, model: str = "google/gemma-3-4b-it", api_key: str = None):
        super().__init__()
        import httpx
        self.httpx = httpx
        self.model = model
        if api_key:
            self.api_key = api_key
        elif os.environ.get("OPENROUTER_API_KEY"):
            self.api_key = os.environ["OPENROUTER_API_KEY"]
        else:
            key_path = Path.home() / ".tokens/openrouter_api_key"
            if key_path.exists():
                self.api_key = key_path.read_text().strip().split()[0]
            else:
                raise ValueError("No OpenRouter API key found. Set OPENROUTER_API_KEY or create ~/.tokens/openrouter_api_key")

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
    client: GeminiClient,
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
    parser = argparse.ArgumentParser(description="V2 Media description pipeline")
    parser.add_argument("--source", choices=["tgif", "r2", "local", "manifest"], default="tgif",
                        help="Data source type")
    parser.add_argument("--tsv", type=Path, default=DEFAULT_TSV,
                        help="Path to TGIF TSV file (for tgif source)")
    parser.add_argument("--r2-bucket", help="R2 bucket name (for r2 source)")
    parser.add_argument("--r2-prefix", default="", help="R2 key prefix filter")
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
    parser.add_argument("--model", default="gemini-3.1-flash-lite-preview",
                        help="Model name (Gemini for vertex/genai, Gemma for termite)")
    parser.add_argument("--backend", choices=["vertex", "genai", "termite", "ollama", "openrouter"], default="vertex",
                        help="API backend: vertex (GCP), genai (API key), termite (local Gemma), ollama (local Gemma), or openrouter")
    parser.add_argument("--batch", action="store_true",
                        help="Use Batch API for genai backend (50%% cheaper, async processing)")
    parser.add_argument("--project", default="honeycomb-488503",
                        help="GCP project ID (for vertex backend)")
    parser.add_argument("--location", default="us-central1",
                        help="GCP region (for vertex backend)")
    parser.add_argument("--termite-url", default="http://localhost:11433",
                        help="Termite API URL (for termite backend)")
    parser.add_argument("--termite-model", default="onnxruntime/Gemma-3-ONNX",
                        help="Termite model name (for termite backend)")
    parser.add_argument("--ollama-url", default="http://localhost:11434",
                        help="Ollama API URL (for ollama backend)")
    parser.add_argument("--ollama-model", default="gemma3:4b-it-qat",
                        help="Ollama model name (for ollama backend)")
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
    if args.source == "tgif":
        source = TGIFSource(args.tsv)
        print(f"Using TGIF source: {args.tsv}")
    elif args.source == "r2":
        if not args.r2_bucket:
            print("Error: --r2-bucket required for r2 source", file=sys.stderr)
            sys.exit(1)
        source = R2Source(args.r2_bucket, args.r2_prefix)
        print(f"Using R2 source: {args.r2_bucket}/{args.r2_prefix}")
    elif args.source == "local":
        if not args.local_dir:
            print("Error: --local-dir required for local source", file=sys.stderr)
            sys.exit(1)
        source = LocalSource(args.local_dir)
        print(f"Using local source: {args.local_dir}")
    elif args.source == "manifest":
        if not args.manifest_file:
            print("Error: --manifest-file required for manifest source", file=sys.stderr)
            sys.exit(1)
        bucket = args.r2_bucket or "honeycomb-media"
        source = ManifestSource(args.manifest_file, bucket)
        print(f"Using manifest source: {args.manifest_file} ({len(source.items_data)} items)")

    # Initialize client based on backend
    if args.backend == "vertex":
        client = VertexAIClient(project=args.project, location=args.location, model=args.model)
        print(f"Using Vertex AI: {args.project}/{args.location}, model: {args.model}")
    elif args.backend == "termite":
        client = TermiteClient(url=args.termite_url, model=args.termite_model)
        print(f"Using Termite: {args.termite_url}, model: {args.termite_model}")
    elif args.backend == "ollama":
        client = OllamaClient(url=args.ollama_url, model=args.ollama_model)
        print(f"Using Ollama: {args.ollama_url}, model: {args.ollama_model}")
    elif args.backend == "openrouter":
        client = OpenRouterClient(model=args.model)
        print(f"Using OpenRouter, model: {args.model}")
    else:
        client = GoogleGenAIClient(model=args.model)
        print(f"Using Google GenAI, model: {args.model}")

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

    # Load model pricing config
    import yaml
    pricing_path = Path(__file__).parent / "models.yaml"
    model_pricing = {}
    if pricing_path.exists():
        with open(pricing_path) as f:
            models_config = yaml.safe_load(f)
        for m in models_config.get("models", []):
            model_pricing[m["id"]] = (m.get("input_per_m", 0), m.get("output_per_m", 0))

    use_batch = args.batch and isinstance(client, GoogleGenAIClient)
    if use_batch:
        print("Using Batch API (50% discount)")

    success = 0
    failed = 0
    start = time.time()
    output_mode = "a" if args.resume else "w"

    if use_batch:
        # Batch mode: download all media items, extract frames, submit as one batch
        print("Downloading and extracting frames...")
        batch_requests = []  # (prompt, images)
        batch_items = []     # corresponding GifItems
        for i, item in enumerate(to_process):
            gif_data = source.download(item)
            if gif_data is None:
                failed += 1
                continue
            try:
                frames = extract_frames(gif_data, num_frames=args.frames)
            except Exception as e:
                print(f"  Frame extraction error for {item.id}: {e}", file=sys.stderr)
                failed += 1
                continue
            batch_requests.append((prompt, frames))
            batch_items.append(item)
            if (i + 1) % 10 == 0:
                print(f"  Prepared {i + 1}/{len(to_process)}")

        print(f"Prepared {len(batch_requests)} requests, submitting batch...")
        responses = client.generate_batch(batch_requests)

        with open(args.output, output_mode) as out:
            for item, response in zip(batch_items, responses):
                if response is None:
                    failed += 1
                    continue
                try:
                    text = clean_json_response(response)
                    data = json.loads(text)
                    data["id"] = item.id
                    data["dataset"] = item.dataset
                    data["source_path"] = item.source_path
                    if item.original_url:
                        data["original_url"] = item.original_url
                    if item.original_description:
                        data["original_description"] = item.original_description
                    if item.attribution:
                        data["attribution"] = item.attribution
                    out.write(json.dumps(data) + "\n")
                    success += 1
                    state.mark_processed(item.id)
                except json.JSONDecodeError as e:
                    print(f"  JSON parse error for {item.id}: {e}", file=sys.stderr)
                    failed += 1
    else:
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

    # Cost estimate
    input_tokens = client.total_input_tokens
    output_tokens = client.total_output_tokens
    token_source = "actual"
    if input_tokens == 0 and success > 0:
        # Fallback estimate if API didn't return usage
        input_tokens = success * 1500
        output_tokens = success * 200
        token_source = "estimated"

    input_price, output_price = model_pricing.get(args.model, (0, 0))
    batch_discount = 0.5 if use_batch else 1.0
    cost = ((input_tokens * input_price / 1_000_000) +
            (output_tokens * output_price / 1_000_000)) * batch_discount
    print(f"Tokens: {input_tokens:,} in / {output_tokens:,} out ({token_source})")
    if input_price > 0 or output_price > 0:
        print(f"Cost: ${cost:.4f} ({args.model}, {'batch' if use_batch else 'standard'})")
    else:
        print(f"Cost: unknown (add {args.model} to models.yaml)")

    # Write summary alongside JSONL for review.py to pick up
    summary_path = args.output.with_suffix(".summary.json")
    summary = {
        "model": args.model,
        "backend": args.backend,
        "batch": use_batch,
        "items": success,
        "failed": failed,
        "input_tokens": input_tokens,
        "output_tokens": output_tokens,
        "token_source": token_source,
        "cost": round(cost, 6),
        "elapsed_s": round(elapsed, 1),
    }
    with open(summary_path, "w") as f:
        json.dump(summary, f, indent=2)
        f.write("\n")


if __name__ == "__main__":
    main()
