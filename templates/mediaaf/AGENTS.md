# MediaAF Agent Guide

MediaAF turns an image or moving-image corpus into a searchable Antfly Cloud table.
It does **not** use multimodal embeddings. The pipeline is:

```text
media file -> transform package -> LLM description JSON -> combined_text -> text embedding in Antfly
```

## Target outcome

A locally hostable MediaAF app where a user can search an image/moving-image corpus by meaning, mood, tags, actions, and use case. The corpus has been transformed into LLM-generated description which is used for text embeddings to enable semantic search. Everything is ingested into an Antfly Cloud table with text embeddings over `combined_text`, and served through a local web UI/proxy without exposing the Cloud API key to the browser.

## Best out-of-box corpus

Prefer visually meaningful, relatively short assets:

- reaction GIFs, stickers, memes, screenshots where visual content matters more than OCR;
- short MP4/WebM clips where a handful of representative frames is enough;
- PNG/JPEG/WebP image libraries;
- product/brand/marketing asset libraries.

Poor default fits unless you add custom transforms:

- long videos, films, lectures, or screen recordings that need scene segmentation;
- podcasts or long audio that need transcript/chapter workflows;
- PPTX/PDF/document corpora, where DocsAF or document-specific preprocessing is usually better;
- OCR-heavy scans where correctness of recognized text is the product.

## Human decisions to lock before running

1. Which media types are in scope for this corpus?
2. Which transform should each media type use?
3. Which description model should summarize the transformed package?
4. Which fields should the UI emphasize (mood, tags, people, usage context, rating, etc.)?
5. Whether content filtering/rating should be conservative.

## Implementation workflow

1. Copy `.env.example` to `.env.local` and fill in Cloud/API/model credentials.
2. Inspect `mediaaf.yaml` and update `media_types` to match the corpus.
3. If the corpus has unsupported media, add or edit a transform under `transforms/`.
4. Run `make describe-sample` before spending money on a full corpus.
5. Run `make describe` to create normalized description JSONL.
6. Run `make ingest` to create/update the Antfly table.
7. Run `make proxy` and `make web` for local search.

## API key safety

Keep `ANTFLYDB_API_KEY` in `.env.local`. The browser UI should talk to the local proxy, not directly to Antfly Cloud with a bundled key. For production, replace the local proxy with a real backend that owns credentials, auth, rate limits, and logs.
