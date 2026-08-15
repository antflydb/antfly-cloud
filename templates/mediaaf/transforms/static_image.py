#!/usr/bin/env python3
"""Build an LLM package for a static image."""
from __future__ import annotations
import argparse, hashlib, json, mimetypes
from pathlib import Path

ap = argparse.ArgumentParser()
ap.add_argument("--input", required=True)
ap.add_argument("--output", required=True)
ap.add_argument("--media-url", default="")
args = ap.parse_args()
path = Path(args.input)
data = path.read_bytes()
package = {
    "media_type": mimetypes.guess_type(path.name)[0] or "application/octet-stream",
    "source": {"filename": path.name, "uri": args.media_url or str(path), "sha256": hashlib.sha256(data).hexdigest()},
    "llm_inputs": [{"type": "image", "path": str(path), "role": "source image"}],
    "hints": {"extraction_strategy": "static_image"},
}
Path(args.output).parent.mkdir(parents=True, exist_ok=True)
Path(args.output).write_text(json.dumps(package, indent=2))
