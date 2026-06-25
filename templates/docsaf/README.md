# DocsAF Cloud Template

A starter pack for building a searchable document corpus on Antfly Cloud using the existing DocsAF CLI/library.

```text
document corpus -> docsaf source rows -> Antfly document extraction -> chunks/vectors/search UI
```

## Quick start

```sh
cp .env.example .env.local
make build-docsaf      # or set DOCSAF_BIN=/path/to/docsaf
make prepare           # inspect source-document rows from sample-corpus
make sync              # create/update the Cloud table
make query-smoke
make proxy
make web
```

See `AGENTS.md` for the implementation guide and production notes.
