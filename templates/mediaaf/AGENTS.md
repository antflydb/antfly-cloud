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
3. Which inference provider and description model should summarize the transformed package?
4. Which fields should the UI emphasize (mood, tags, people, usage context, rating, etc.)?
5. Whether content filtering/rating should be conservative.
6. Whether the user is comfortable starting any corpus-wide, expensive description run.

## Inference provider policy

Prefer inference through Antfly Cloud when the user has access to an instance with
an attached inference proxy. Treat Cloud inference as real and available until you
have checked the local account/instance context and proven otherwise.

Provider preference order:

1. User-selected provider/config. If the user names a provider or model, use it.
2. Antfly Cloud inference, when the selected Cloud instance exposes an inference
   proxy that supports the media-description request. Discover the URL with
   `antfly cloud connection <instance> --json` and use
   `antfly_inference_proxy_url`.
3. A direct hosted model provider, such as Gemini or OpenRouter, only when the
   user chooses it and provides credentials. If credentials are missing, stop and
   ask the user.

Do not substitute local inference or older model families just because they are
available. This template should be a simple pattern to adapt, not a provider
compatibility matrix.

Before running a full corpus-wide description job, prefer to confirm with the
user that the provider, model, and expected cost/runtime are acceptable. Always
run a small sample first.

## Implementation workflow

1. Copy `.env.example` to `.env.local` and fill in Cloud/API/model credentials.
2. Inspect `mediaaf.yaml` and update `media_types` to match the corpus.
3. Choose `DESCRIPTION_PROVIDER` explicitly. Prefer `antfly` if accessible.
4. If the corpus has unsupported media, add or edit a transform under `transforms/`.
5. Run `make describe-sample` before spending money on a full corpus.
6. Run `make describe` to create normalized description JSONL.
7. Run `make ingest` to create/update the Antfly table.
8. Run `make proxy` and `make web` for local search.

## API key safety

Keep `ANTFLYDB_API_KEY` in `.env.local`. The browser UI should talk to the local proxy, not directly to Antfly Cloud with a bundled key. For production, replace the local proxy with a real backend that owns credentials, auth, rate limits, and logs.

## About page customization

The starter About page is intentionally generic. After the corpus and provider
choices are real, update it with specific dataset names, item counts, model
choices, or attribution details that are true for this project.
