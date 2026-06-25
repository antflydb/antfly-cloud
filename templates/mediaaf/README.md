# MediaAF

MediaAF is an Antfly Cloud starter pack for searching image and moving-image corpora by meaning, mood, tags, actions, and use case.

MediaAF does **not** use multimodal embeddings. It transforms each media object into a package a multimodal LLM can summarize, stores the normalized text/metadata, and uses Antfly text embeddings over `combined_text` for semantic search.

## Best fit

Good out of the box: reaction GIFs, memes, stickers, short clips, screenshots where visual content matters, and image asset libraries.

Not a great default fit: long videos, podcasts, OCR-heavy archives, PDFs, PPTX, or document corpora. Those can be supported by custom transforms, but DocsAF or a domain-specific workflow is usually better.

## Pipeline

1. Discover or stage media.
2. Transform each media item according to `mediaaf.yaml`.
3. Ask the selected description model for rich JSON metadata: literal description, mood, tags, actions, visual style, rating, use case. Prefer Antfly Cloud inference when your account/instance exposes it; otherwise choose a provider explicitly.
4. Ingest descriptions into Antfly Cloud with a text embedding index on `combined_text`.
5. Search locally through the web UI and local proxy.

## Quick start

```sh
cp .env.example .env.local
# Fill DESCRIPTION_PROVIDER, DESCRIPTION_MODEL, and provider credentials.
# Prefer ANTFLY_INFERENCE_URL from: antfly cloud connection <instance> --json
make setup
make describe-sample
make ingest
make proxy
make web
```

Run `make` to list available commands. See `AGENTS.md` for the implementation guide.
