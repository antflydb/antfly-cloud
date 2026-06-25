# DocsAF Agent Guide

DocsAF turns a document corpus into source-document rows in Antfly Cloud. Antfly then owns extraction, units, chunks, embeddings, full-text indexes, and document hierarchy artifacts.

This template is a thin Cloud starter around the existing DocsAF implementation. Do not re-home or fork the DocsAF library unless the human explicitly asks for that. Build or locate the `docsaf` CLI, then use it from this template.

## Target outcome

A locally hostable document search app backed by an Antfly Cloud table populated by DocsAF source-document rows. Antfly has derived document units/chunks/vectors from the source documents, and the user can search the corpus through a minimal UI or query smoke script.

## Best out-of-box corpus

Good fits:

- Markdown/MDX docs
- HTML documentation exports
- PDFs and slide decks that Antfly can fetch or that are small enough to inline for smoke tests
- OpenAPI/spec/documentation bundles
- Google Drive folders, when using DocsAF's Drive source support

Use MediaAF instead for reaction/image/video libraries. Use Cloud structured import Jobs for CSV/JSON/NDJSON records or sidecar metadata that is already structured.

## Human decisions to lock before running

1. Which corpus source: local directory, Google Drive, or fetchable S3/HTTPS URLs?
2. Which Antfly Cloud instance and table name?
3. For local/private docs, whether `--inline-content` is acceptable for the corpus size.
4. For production, where source documents will live so Antfly can fetch them directly.
5. Desired chunk size/overlap and embedding model, if defaults are not acceptable.

## Workflow

1. Copy `.env.example` to `.env.local` and fill in Cloud settings.
2. Build or point to a `docsaf` binary with `make build-docsaf` or `DOCSAF_BIN=/path/to/docsaf`.
3. Run `make prepare` against `sample-corpus` or a real corpus to inspect source rows.
4. Run `make sync` to create/update the Cloud table.
5. Run `make query-smoke` and `make web` to inspect search behavior.

## Jobs note

Cloud import Jobs are excellent for structured object imports. DocsAF's default path is direct `docsaf sync` because it creates source-document rows and lets Antfly derive document units/chunks/vectors. Do not replace this with structured import Jobs unless the project intentionally wants row import instead of document hierarchy.

## API key safety

Keep `ANTFLYDB_API_KEY` in `.env.local`. The browser UI uses `scripts/local_proxy.py` so the key stays server-side on your machine.
