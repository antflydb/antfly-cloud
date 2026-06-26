#!/usr/bin/env python3
"""Build an LLM package by sampling representative frames from GIF/video media.

GIFs are handled with Pillow. Video files require ffmpeg on PATH.
"""
from __future__ import annotations
import argparse, hashlib, json, mimetypes, subprocess, tempfile
from pathlib import Path
from PIL import Image, ImageSequence

ap = argparse.ArgumentParser()
ap.add_argument("--input", required=True)
ap.add_argument("--output", required=True)
ap.add_argument("--frames", type=int, default=5)
ap.add_argument("--media-url", default="")
args = ap.parse_args()
path = Path(args.input)
out = Path(args.output)
artifacts = out.parent / (out.stem + "-frames")
artifacts.mkdir(parents=True, exist_ok=True)
media_type = mimetypes.guess_type(path.name)[0] or "application/octet-stream"
frame_paths: list[Path] = []

if media_type == "image/gif" or path.suffix.lower() == ".gif":
    im = Image.open(path)
    frames = [f.copy().convert("RGB") for f in ImageSequence.Iterator(im)]
    if frames:
        indexes = sorted(set(round(i * (len(frames) - 1) / max(args.frames - 1, 1)) for i in range(args.frames)))
        for n, idx in enumerate(indexes):
            fp = artifacts / f"frame-{n:03d}.jpg"
            frames[idx].thumbnail((1024, 1024))
            frames[idx].save(fp, quality=90)
            frame_paths.append(fp)
else:
    # Sample frames across the whole clip. This is intentionally simple; long videos
    # should use a custom transform with scene segmentation/transcripts.
    pattern = artifacts / "frame-%03d.jpg"
    subprocess.run([
        "ffmpeg", "-y", "-i", str(path), "-vf", "fps=1,scale='min(1024,iw)':-2", "-frames:v", str(args.frames), str(pattern)
    ], check=True, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    frame_paths = sorted(artifacts.glob("frame-*.jpg"))

data = path.read_bytes()
package = {
    "media_type": media_type,
    "source": {"filename": path.name, "uri": args.media_url or str(path), "sha256": hashlib.sha256(data).hexdigest()},
    "llm_inputs": [{"type": "image", "path": str(p), "role": f"representative frame {i+1} of {len(frame_paths)}"} for i, p in enumerate(frame_paths)],
    "hints": {"extraction_strategy": f"{len(frame_paths)}_representative_frames", "not_for_long_video": True},
}
out.parent.mkdir(parents=True, exist_ok=True)
out.write_text(json.dumps(package, indent=2))
