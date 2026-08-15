# Antfly Cloud Templates

Templates are starter packs for building useful projects on top of Antfly Cloud.
They are different from low-level Antfly examples:

- use `antfly/examples` for examples that can run entirely against local Antfly or Antfly Lite;
- use `antfly-cloud/templates` when the project assumes Cloud account concepts such as hosted instances, Cloud URLs, API keys, provider auths, import jobs, or RBAC.

Each template is intentionally `AGENTS.md`-first. The `AGENTS.md` file is the launch guide an implementing agent should follow with a human: it explains the project shape, what inputs are needed, which local files are safe to edit, and where the Cloud API key belongs.

## Templates

- [`mediaaf`](./mediaaf/) — image and moving-image search by generating rich text descriptions, then embedding those descriptions in Antfly.
- [`docsaf`](./docsaf/) — document-corpus search using the existing DocsAF source-document flow and Antfly-managed extraction/chunk/vector hierarchy.

## Local API key pattern

These templates are locally hostable by default. Keep Antfly API keys in a gitignored local environment file, and have scripts or a local proxy read them server-side:

```sh
cp .env.example .env.local
# edit .env.local
```

Static browser apps should not embed Antfly Cloud API keys. For local development, run the template proxy and point the UI at `http://127.0.0.1:<port>`. For production, replace the local proxy with a real backend, scoped credentials, user auth, logging, rate limits, and secret management.
