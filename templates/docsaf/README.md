# DocsAF Cloud Template

A starter pack for building a searchable document corpus on Antfly Cloud using the existing DocsAF CLI/library.

```text
document corpus -> docsaf source rows -> Antfly document extraction -> chunks/vectors/search UI
```

## Implementation

Start by reading `AGENTS.md`. This template intentionally does not include a Makefile: the agent should locate the existing DocsAF CLI, adapt the corpus source, and then use the included smoke-test/proxy/UI assets as needed.
