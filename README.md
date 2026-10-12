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

## Document import and retrieval

The Cloud CLI is also available as `antfly cloud` when `antfly-cloud` is installed
alongside Antfly. These commands use an **existing instance**; they do not provision
or upgrade one. Authenticate with `antfly cloud login`, then select its organization
and instance using `--org` and `--instance` or saved defaults.

```sh
antfly cloud --org acme --instance prod --json import capabilities
antfly cloud --org acme --instance prod --json provider-auth create --file s3-auth.json
antfly cloud --org acme --instance prod --json provider-auth list
antfly cloud --org acme --instance prod --json provider-auth objects AUTH_ID --bucket docs --prefix manuals/ --limit 20
antfly cloud --org acme --instance prod --json import create --file import.json
antfly cloud --org acme --instance prod --json import run JOB_ID
antfly cloud --org acme --instance prod --json import wait JOB_ID RUN_ID --wait-timeout 30m
antfly cloud --org acme --instance prod --json import runs JOB_ID --limit 20 --offset 0
antfly cloud --org acme --instance prod --json import health JOB_ID
antfly cloud --org acme --instance prod --json import retry-failed JOB_ID
```

`create --file PATH` accepts request JSON; `--file -` reads stdin. Provider auth
requests contain `provider`, `display_name`, `auth_type`, `secret`, `config`, and
optional `enabled`/`is_default`. For S3, `provider` is `s3`, `auth_type` is
`access_key_pair`, and `secret` is a JSON-encoded **string** containing
`access_key_id` and `secret_access_key` (optional `session_token`). Configuration
contains public endpoint, region, bucket allowlist, and TLS settings. Do not
commit credential files or include secrets in command arguments.

Import request JSON accepts `name`, `provider_auth_id`, `target_table`, `mode`,
`source_config`, `mapping_config`, `id_field`, `document_id_template`, `run_policy`,
and `schedule_interval_seconds`. Source and mapping JSON are preserved unchanged.
Consult `import capabilities` for supported modes, formats, and retryable codes.

Import and provider-auth commands emit JSON even without `--json`; lists are
arrays, other responses are objects. Runs retain source-level diagnostics and
coverage counters. Capture the `id` returned by `run` or `retry-failed` and wait
for **that specific run**. `wait` searches paginated history and exits nonzero
for partial/failed outcomes, missing/unknown outcomes, timeout, or cancellation.
It prints the last observed run on failure. A `complete` status alone is not
success. `--timeout` controls each API request; `--wait-timeout` controls the
overall wait, and `--poll-interval` defaults to two seconds. Ctrl-C stops waiting
without canceling the backend run.

Use `import pause JOB_ID`, `import resume JOB_ID`, or `import archive JOB_ID` to
manage scheduling and visibility. Archive retains history; `import list
--include-archived` includes archived jobs. Pause does not disable manual runs.

Retrieve imported documents via the existing OAuth connection:

```sh
antfly cloud --org acme --instance prod --json instance table get TABLE_NAME
antfly cloud --org acme --instance prod --json instance query --table TABLE_NAME --file query.json
# query.json: {"full_text_search":{"query":"installation"},"full_text_index":"document_text","hierarchy":{}}
```

Query JSON is sent to the named-table data-plane route, including index selectors
and hierarchy options. These commands use the same connection/token as `antfly
cloud env`; management API keys cannot access the data plane. To use existing
Antfly data-plane commands, run `eval "$(antfly cloud env)"`.

Create a table-scoped reader with `instance api-key create --file reader.json`:

```json
{"name":"document-reader","key_type":"read_only","grants":[{"table_name":"TABLE_NAME","actions":["table.read"]}]}
```

Successful key creation intentionally prints the secret once (the `key` field
in `--json` output). Store that output securely. Credential errors omit server
response bodies to avoid exposing secrets; HTTP status is retained. Provider
auth output never includes its secret material.

Clean up only your own resources:

```sh
antfly cloud --org acme --instance prod --json instance api-key revoke KEY_ID
antfly cloud --org acme --instance prod --json import archive JOB_ID
antfly cloud --org acme --instance prod --json provider-auth revoke AUTH_ID
antfly cloud --org acme --instance prod --json instance table drop TABLE_NAME
```

Dropping a table permanently deletes its documents. No bulk/all-table operation is
provided. Finish active runs before revoking credentials. Provider-auth revocation
disables the Cloud credential and permanently clears its stored secret, but does
not revoke the key at the upstream provider. Metadata remains for archived job
references; `provider-auth delete` is only for credentials with no references.


## Development

```sh
make test
```

The SDKs are generated from `openapi.yaml`. Colony is responsible for producing
the public API bundle; this repository is responsible for validating and
releasing public clients.

