# Managed Lite snapshot workflow

This client workflow requires server-admitted Lite capabilities. The presence
of these commands does not mean managed Lite is available in an organization.
Use `antfly-cloud --org ORG lite capabilities` to check. Existing billing
admission and API permissions apply; this document introduces no price or
availability guarantee.

A Lite file is one managed database with its own endpoint. For the currently
qualified embedded-root shape, its table is `default`. Transfers create
independent snapshots, not replicas. Use a new destination for every import
and download. Preserve your original database until the new copy is verified.

## Import and explicit cutover

For live application cutover, pause writes **before** taking the snapshot and
keep them paused through verification and the deliberate application switch.
If you only want a copy, local writes may continue, but later writes are not
part of that snapshot.

```sh
antfly lite snapshot local.aflite --out upload.aflite
antfly-cloud --org ORG lite import upload.aflite --idempotency-key local-copy-001
antfly-cloud --org ORG --json lite status IMPORT_OPERATION_ID
```

Keep the same snapshot and idempotency key to resume an interrupted upload.
The client hashes and transfers in bounded chunks. It queries the storage
service's committed offset after ambiguous responses. Never copy or share an
upload URL; it grants access to that upload.

Review the `awaiting_confirmation` report, especially dependency requirements.
Unsupported or unknown model/media dependencies must be resolved before
creation. Stored vectors do not establish that automatic query embedding or
future ingestion will work. The server must qualify the exact model identity,
preprocessing and dimensions; it must not silently substitute another model.

```sh
antfly-cloud --org ORG lite confirm IMPORT_OPERATION_ID \
  --revision REVIEWED_REVISION --name 'My Lite database' --slug my-lite \
  --acknowledge-snapshot-boundary --accept-lite-terms
antfly-cloud --org ORG lite status IMPORT_OPERATION_ID
```

The current Lite package uses a fixed 50 GiB disk. The uploaded source and an
independent snapshot need at least 1 GiB of additional headroom, limiting the
current import path to 24.5 GiB before expected growth. The server checks capacity
and billing admission before creating the database. Review the server's current
plan and explicitly accept its terms. Request `--claim-free-year` only when the
capabilities report says the organization is eligible; that request never falls
back to paid creation if the offer becomes unavailable. Confirmation
retries with the same request return the original database. A different
request cannot bind the same operation to a second database.

Get the instance connection details and create a scoped **data API key** using
the existing Cloud connection and key commands. A management token authorizes
Cloud operations; it is not sent to the data endpoint or object storage.
Before switching the application, compare IDs/counts and selected edits, query
existing indexes, exercise automatic query embedding, and ingest a new record.
Resume writes against the chosen endpoint only after that verification.

## One application client on both sides

Start with HTTP-served Lite if you want to use the same network SDK locally and
in Cloud:

```sh
antfly standalone --storage-engine lite --storage-path local.aflite \
  --data-dir ./local-runtime --host 127.0.0.1 --port 8080 --fsync true
```

Configure inference/model paths and secrets explicitly for your database's
requirements. Then use the official Python SDK:

```python
import os
from antfly import AntflyClient

local = AntflyClient(base_url='http://127.0.0.1:8080/db/v1')
managed = AntflyClient(
    base_url=os.environ['ANTFLY_DATABASE_URL'],
    api_key=(os.environ['ANTFLY_KEY_ID'], os.environ['ANTFLY_KEY_SECRET']),
)
for db in (local, managed):
    result = db.query('default', full_text_search={'match': 'portability', 'field': 'body'})
    print(result.to_dict())
```

An embedded file binding has different lifecycle, transaction, query and result
interfaces. Such applications need an explicit adapter or code changes; merely
changing an endpoint does not convert an embedded connection into an HTTP
client.

## Sleep and resume

After 30 days without customer data access, an admitted Lite database can stop
serving compute while retaining its disk. Queries do not wake or replay work.
Resume explicitly, then wait for the instance to become ready before retrying:

```sh
antfly-cloud --org ORG lite resume my-lite
```

The package remains billable while asleep after any introductory grant expires.
Billing suspension must be resolved before resuming.

## Export and return

Pause writes first if this is a live cutover. The export retains the first
consistent snapshot across retries; later Cloud writes are excluded.

```sh
antfly-cloud --org ORG lite export my-lite --idempotency-key return-copy-001
antfly-cloud --org ORG lite download EXPORT_OPERATION_ID --out returned.aflite
antfly restore --preflight --input returned.aflite \
  --destination-capabilities laptop-capabilities.json
antfly standalone --storage-engine lite --storage-path returned.aflite \
  --data-dir ./returned-runtime --host 127.0.0.1 --port 8081 --fsync true
```

The CLI resumes interrupted downloads, refreshes expired download links while
preserving source identity, verifies SHA-256, and publishes only to a new file.
It never replaces an existing destination. Configure the local runtime to meet
the preflight report; remote inference/media are allowed when explicitly
configured and compatible. Recheck queries and ingestion before changing the
application endpoint. The returned database is independent of the managed
writer.

The browser can upload in bounded chunks and resume with its transfer key. Its
native download streams to disk without a whole-file JavaScript buffer. Use
the CLI for automatic download verification, resumption and create-only local
publication, especially for large databases.

An unconfirmed import or pending export can be cancelled with
`antfly-cloud --org ORG lite cancel OPERATION_ID`. Cancellation does not delete
an already created database. Cleanup is asynchronous; quotas are released only
after workers and storage capabilities are drained and temporary objects are
verified deleted.
