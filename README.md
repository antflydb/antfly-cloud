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
antfly cloud --org acme --instance prod --json connections create --provider s3 --file s3-auth.json
antfly cloud --org acme --instance prod --json connections list
antfly cloud --org acme --instance prod --json connections get CONNECTION_ID
antfly cloud --org acme --instance prod --json connections objects CONNECTION_ID --bucket docs --prefix manuals/ --limit 20
antfly cloud --org acme --instance prod --json import create --connection CONNECTION_ID --table TABLE_NAME --file import.json
antfly cloud --org acme --instance prod --json import run IMPORT_ID
antfly cloud --org acme --instance prod --json import wait IMPORT_ID RUN_ID --wait-timeout 30m
antfly cloud --org acme --instance prod --json import runs IMPORT_ID --limit 20 --offset 0
antfly cloud --org acme --instance prod --json import health IMPORT_ID
antfly cloud --org acme --instance prod --json import retry-failed IMPORT_ID
```

Cloud `connections` manages the hosted credential vault on the selected instance.
This is distinct from core Antfly's named connections with capabilities and secret
references: the hosted API uses the request schemas below, not the core connection
schema or custom secret-reference syntax. Local `antfly connections login aws` or
`antfly connections login google` does not upload credentials to Cloud.

`create --file PATH` accepts request JSON; `--file -` reads stdin. Hosted connection
requests contain `provider`, `display_name`, `auth_type`, `secret`, `config`, and
optional `enabled`/`is_default`. `connections create --provider` may supply
`provider` instead of the file. For S3, `provider` is `s3`, `auth_type` is
`access_key_pair`, and `secret` is a JSON-encoded **string** containing
`access_key_id` and `secret_access_key` (optional `session_token`). Configuration
contains public endpoint, region, bucket allowlist, and TLS settings. Do not
commit credential files or include secrets in command arguments.

Import request JSON accepts `name`, `provider_auth_id`, `target_table`, `mode`,
`source_config`, `mapping_config`, `id_field`, `document_id_template`, `run_policy`,
and `schedule_interval_seconds`. `import create --connection` supplies
`provider_auth_id`; `--table` supplies `target_table`. These JSON wire field names
are unchanged. Explicitly empty flags and flags conflicting with nonempty JSON
fields are rejected before any HTTP request; equal values, file-only selectors,
and flag-only selectors are accepted. Source and mapping JSON are preserved
unchanged. Consult `import capabilities` for supported modes, formats, and
retryable codes.

Import and connections commands emit JSON even without `--json`; lists are
arrays, other responses are objects. Runs retain source-level diagnostics and
coverage counters. Capture the `id` returned by `run` or `retry-failed` and wait
for **that specific run**. `wait` searches paginated history and exits nonzero
for partial/failed outcomes, missing/unknown outcomes, timeout, or cancellation.
It prints the last observed run on failure. A `complete` status alone is not
success. `--timeout` controls each API request; `--wait-timeout` controls the
overall wait, and `--poll-interval` defaults to two seconds. Ctrl-C stops waiting
without canceling the backend run.

Use `import pause IMPORT_ID`, `import resume IMPORT_ID`, or `import archive IMPORT_ID` to
manage scheduling and visibility. Archive retains history; `import list
--include-archived` includes archived imports. Pause does not disable manual runs.

### Breaking imports API upgrade

Upgrade the Cloud service and clients together. The public resource is now
`/organizations/{org_id}/cloud/instances/{instance_id}/imports`, with execution
history at `/imports/{import_id}/runs`. Run and health responses identify their
saved import with `import_id`; each resource's own `id` and nested `last_run`
remain unchanged. Update raw HTTP paths and response decoders before upgrading.
There are no compatibility routes, redirects, deprecated wrappers, or aliases.

Go SDK callers must use `CreateImportRequest`, `Import`, and `ImportRun`, and read
`ImportRun.ImportID` and `DocumentTableHealth.ImportID`. The exported methods are
now `Imports`, `CreateImport`, `ImportRuns`, `ImportHealth`, `RunImport`,
`PauseImport`, `ResumeImport`, `ArchiveImport`, and `WaitImportRun`.
`ImportRunResult` takes `ImportRun`. The CLI keeps its `import` group, actions,
and flags; import arguments are `<import-id>`, and `wait` still takes the
specified `<run-id>`. Existing saved imports and run history are retained.

### Inspecting and rotating hosted credentials

`connections get CONNECTION_ID` returns metadata, never the stored secret. To
rotate a secret, prepare an update file and pass it via `--file PATH` or stdin:

```sh
antfly cloud --org acme --instance prod --json connections update CONNECTION_ID --file rotation.json
# rotation.json: {"secret":"<provider credential string>","replace_secret":true}
antfly cloud --org acme --instance prod --json connections validate CONNECTION_ID
```

Update JSON accepts `display_name`, `secret`, `replace_secret`, `clear_secret`,
`config`, `enabled`, and `is_default`. There is no writable `status` field.
Empty updates are rejected. A replacement requires both a nonblank `secret` and
`replace_secret: true`; it cannot also request `clear_secret: true`. To clear only,
use `{"clear_secret":true}`. Explicit `false` values are meaningful, including
`enabled: false`; validation does not re-enable a disabled connection.

A secret replacement is first stored by PATCH, then provider validation is
requested on the same connection ID. Only an `active` validation response means
the command succeeds. On validation failure the command exits nonzero but prints
the retained PATCH metadata, including its ID/state, without secret material.
The replacement is not rolled back. After correcting the provider-side problem,
use `connections validate CONNECTION_ID` for recovery; it also exits nonzero for
an HTTP failure or a status other than `active`. Metadata-only and clear-only
updates do not automatically validate.

Stored replacement, provider validation, and asynchronous runtime reconciliation
are separate steps. These endpoints do not prove that the instance runtime has
adopted the replacement, even when validation succeeds. Rotate outside active
imports; uninterrupted active runs are not guaranteed. No runtime-readiness
polling is performed.

### Retrieving documents and managing access

Retrieve imported documents via the existing OAuth data-plane access:

```sh
antfly cloud --org acme --instance prod --json instance endpoints prod
antfly cloud --org acme --instance prod --json instance table get --table TABLE_NAME
antfly cloud --org acme --instance prod --json instance query --table TABLE_NAME --file query.json
# query.json: {"full_text_search":{"query":"installation"},"full_text_index":"document_text","hierarchy":{}}
```

`instance endpoints` inspects status, database proxy, and inference URLs; it does
not manage hosted connections. Table get/drop require `--table` and accept no
positional table name. Query JSON is sent to the named-table data-plane route,
including index selectors and hierarchy options. These commands use the same
endpoint/token as `antfly cloud env`; management API keys cannot access the data
plane. To use existing Antfly data-plane commands, run `eval "$(antfly cloud env)"`.

Create a table-scoped reader with `instance api-key create --file reader.json`:

```json
{"name":"document-reader","key_type":"read_only","grants":[{"table_name":"TABLE_NAME","actions":["table.read"]}]}
```

Each supplied grant requires an explicit nonblank `table_name`; use `*` for all
tables. Omitting `grants` or supplying an empty grants array deliberately preserves
the existing instance-wide key behavior. Unknown typed JSON fields, including
misspelled grant fields, are rejected before authentication or HTTP requests,
rather than silently widening access. Request files must contain exactly one
non-null JSON object.

Successful key creation intentionally prints the secret once (the `key` field
in `--json` output). Store that output securely. Credential errors omit server
response bodies to avoid exposing secrets; HTTP status is retained. Hosted
connection output never includes its secret material.

Clean up only your own resources:

```sh
antfly cloud --org acme --instance prod --json instance api-key revoke KEY_ID
antfly cloud --org acme --instance prod --json import archive IMPORT_ID
antfly cloud --org acme --instance prod --json connections revoke CONNECTION_ID
antfly cloud --org acme --instance prod --json instance table drop --table TABLE_NAME
```

Dropping a table permanently deletes its documents. No bulk/all-table operation is
provided. Finish active runs before revoking credentials. `connections revoke`
disables the hosted credential and permanently clears its stored secret, but does
not revoke the key at the upstream provider. Metadata remains for archived import
references; `connections delete` is only for unreferenced metadata.


## Development

```sh
make test
```

The SDKs are generated from `openapi.yaml`. Colony is responsible for producing
the public API bundle; this repository is responsible for validating and
releasing public clients.

