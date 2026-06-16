# Antfly Cloud

Public CLI, SDKs, examples, and API contract for Antfly Cloud.

The source of truth for the Antfly Cloud service implementation lives in the
private Colony repository. This repository owns public client code and public
release artifacts.

## Layout

```text
openapi.yaml          Public Antfly Cloud OpenAPI bundle
go/                  Go CLI and SDK packages
ts/                  TypeScript packages
examples/            SDK usage examples
docs/                Public client documentation
```

## Packages

- TypeScript SDK: `@antfly/cloud-sdk`
- Go SDK import path: `github.com/antflydb/antfly-cloud/go/pkg/sdk`
- CLI command: `antfly-cloud`

## Development

```sh
make test
```

The SDKs are generated from `openapi.yaml`. Colony is responsible for producing
the public API bundle; this repository is responsible for validating and
releasing public clients.

