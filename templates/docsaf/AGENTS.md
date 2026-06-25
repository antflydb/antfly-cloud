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
2. Build or locate the existing `docsaf` CLI from the Antfly repo; do not fork or re-home DocsAF.
3. Run `docsaf prepare` against `sample-corpus` or a real corpus to inspect source-document rows.
4. Run `docsaf sync` with the Cloud URL/token/table to create or update the Cloud table.
5. Use `python3 scripts/query_smoke.py` or the static UI in `web/` to inspect search behavior.

Example shape, to adapt after locating `docsaf`:

```sh
DOCSAF_BIN=/path/to/docsaf
$DOCSAF_BIN prepare --dir sample-corpus --inline-content --output artifacts/docsaf-source-documents.json
$DOCSAF_BIN sync --url "$ANTFLY_URL" --token "$ANTFLYDB_API_KEY" --table docsaf --create-table --dir sample-corpus --inline-content
python3 scripts/query_smoke.py
python3 scripts/local_proxy.py
python3 -m http.server 8770 --directory web
```

## Jobs note

Cloud import Jobs are excellent for structured object imports. DocsAF's default path is direct `docsaf sync` because it creates source-document rows and lets Antfly derive document units/chunks/vectors. Do not replace this with structured import Jobs unless the project intentionally wants row import instead of document hierarchy.

## API key safety

Keep `ANTFLYDB_API_KEY` in `.env.local`. The browser UI uses `scripts/local_proxy.py` so the key stays server-side on your machine.
