# Manage sources from the CLI

`antfly-cloud sources` manages connected sources through the shared Sources
management API for the selected organization and instance. It uses your Cloud
login; provider credentials stay with the hosted service and are never
downloaded. Hosted runs execute on the server and continue if the CLI exits.

Every command accepts the usual `--org`, `--instance`, `--json` and `--timeout`
flags. Run `antfly-cloud sources --help` (or `--help` on any subcommand) to see
what is available. Sources can be referenced by ID or by unique name.

## Inspect

```sh
antfly-cloud sources capabilities          # what this runtime supports
antfly-cloud sources list --all            # paginated; --limit/--cursor for one page
antfly-cloud sources status engineering-docs
antfly-cloud sources processing status engineering-docs
antfly-cloud sources artifacts list engineering-docs
```

`status` reports acquisition (the latest run) and search readiness separately. A
completed acquisition does not mean the content is searchable; check
`processing status` for each stream and artifact.

## Runs

```sh
antfly-cloud sources run engineering-docs --wait --wait-timeout 20m
antfly-cloud sources runs list engineering-docs
antfly-cloud sources runs inspect engineering-docs RUN_ID
antfly-cloud sources runs wait engineering-docs RUN_ID
antfly-cloud sources runs cancel engineering-docs RUN_ID
```

Each submission carries an idempotency key. Pass `--idempotency-key KEY` to retry
the same submission safely without starting a duplicate run. `--wait` watches
the hosted run; a timeout or Ctrl-C only stops watching and prints the command to
resume. Cancellation is always explicit. `sources sync` is a deprecated alias of
`sources run`. `sources pause|resume` pause or resume a source's hosted schedule.

## Declare sources in a file

```yaml
version: 1
sources:
  - key: engineering-docs          # stable identity; never change it
    name: Engineering docs         # display name; safe to edit
    connection: company-google     # connection ID or unique display name
    table: knowledge               # immutable after creation
    config:
      resource: drive
      folder_id: "FOLDER_ID"
    # Optional: publication, permissions, permission_ttl_seconds,
    # search_version, streams, artifacts, processing, knowledge, identity.
```

```sh
antfly-cloud sources plan --file sources.yaml
antfly-cloud sources apply --file sources.yaml
```

- The source ID is derived from the instance and `key`, so the same file always
  targets the same sources and editing `name` updates in place.
- `plan` validates the file and the server's capabilities and never writes.
- `apply` writes only sources that changed. Reapplying an unchanged file does
  nothing, so content is not reacquired or reprocessed.
- Fields you omit keep their current server values. New sources default to native
  publication and access private to you unless `permissions` is declared.
- Unknown fields are rejected. Streams, artifacts, processing and knowledge use the
  shared declaration schemas.
- `apply` sends each update with the revision it planned against. If someone else
  edited the source in between, that source reports a conflict (exit 5) and is
  not overwritten; review with `plan` and apply again.
- Results are reported per source. If some fail, rerunning `apply` is safe.

## Retire sources

Removing an entry from the file never deletes anything.

```sh
antfly-cloud sources archive engineering-docs      # stop runs; keep indexed content
antfly-cloud sources purge engineering-docs --yes  # delete this source's indexed records
```

`purge` records a purge intent and submits a server-side purge run. It deletes
only records owned by that source. If the run cannot be submitted, the intent is
kept and the same command can be retried.

## Connections

`antfly-cloud connections` manages external provider connections (Google
Workspace, Slack, Discord). The singular `antfly-cloud connection` shows an
instance's endpoint URLs and is unrelated.

```sh
antfly-cloud connections connect google-workspace --name company-google
antfly-cloud connections list
antfly-cloud connections validate company-google
antfly-cloud connections rename company-google eng-google
```

## Scripting

With `--json`, output is machine-readable and errors are a single JSON line on
stderr: `{"error":{"code":"...","status":404,"server_code":"...","message":"..."}}`.

| Exit | Meaning |
|------|---------|
| 0 | success |
| 1 | other failure |
| 2 | invalid input, manifest or ambiguous reference |
| 3 | not authenticated or not authorized |
| 4 | not found |
| 5 | conflict (for example an intervening edit) |
| 6 | not supported by this runtime |
| 7 | timed out (the hosted run continues) |
| 8 | the watched run failed or was cancelled |
| 130 | watching interrupted (the hosted run continues) |
