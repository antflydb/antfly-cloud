# MediaAF

MediaAF is an Antfly Cloud starter pack for searching image and moving-image corpora by meaning, mood, tags, actions, and use case.

MediaAF does **not** use multimodal embeddings. It transforms each media object into a package a multimodal LLM can summarize, stores the normalized text/metadata, and uses Antfly text embeddings over `combined_text` for semantic search.

## Best fit

Good out of the box: reaction GIFs, memes, stickers, short clips, screenshots where visual content matters, and image asset libraries.

Not a great default fit: long videos, podcasts, OCR-heavy archives, PDFs, PPTX, or document corpora. Those can be supported by custom transforms, but DocsAF or a domain-specific workflow is usually better.

## Pipeline

1. Start from a provided local corpus or manifest.
2. Transform each media item according to `mediaaf.yaml`.
3. Ask the selected description model for rich JSON metadata: literal description, mood, tags, actions, visual style, rating, and use case. Prefer Antfly Cloud inference when your account/instance exposes it; otherwise choose a provider explicitly.
4. Ingest descriptions into Antfly Cloud with a text embedding index on `combined_text`.
5. Search locally through the React UI and local proxy. The proxy keeps the Cloud API key out of browser code.

## Implementation

Start by reading `AGENTS.md`. The included transforms, description script, ingest script, proxy, and web app are starter assets for an agent to adapt to the user's corpus and run one working local search path.
