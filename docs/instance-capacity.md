# Managed instance capacity from the CLI

New advanced deployments require Antfly's employee-controlled organization
admission, instance-provision RBAC, and the existing billing/credit checks.
Client flags cannot enable that admission. Antfly staff can manage it through
the private Colony `platform enterprise-deployments` command, using Bugeye's
existing superadmin API. Existing instances retain their normal management path.

Inspect the exact request's live estimate before creating a dedicated instance:

```sh
antfly-cloud --org ORG instance create --request examples/grepstein-instance.json --estimate
antfly-cloud --org ORG instance create --request examples/grepstein-instance.json
antfly-cloud --org ORG instance get grepstein
antfly-cloud --org ORG instance events grepstein
```

`--estimate` does not create anything. Creation always obtains a fresh estimate
and refuses a denied estimate before sending the create request. The server
rechecks admission, capacity entitlements, and billing during creation. A create
response is not proof that provisioning or indexing has finished. API failures
are surfaced; creation is not automatically retried after an ambiguous response.
Check `instance get` by slug before attempting another create.

The Grepstein example requests one Standard node with 8 CPU, 32 GiB memory, and
200 GiB disk. It is a qualification candidate, not a measured capacity guarantee
or a new tier. The estimate reports current committed-capacity pricing; inspect
it in the target organization before starting recurring capacity.

Scale through the same managed instance service, with its normal storage and
entitlement rules:

```sh
antfly-cloud --org ORG instance resize grepstein --request examples/grepstein-resize.json --estimate
antfly-cloud --org ORG instance resize grepstein --request examples/grepstein-resize.json
```

Resize requests contain only `tier`, `mode`, and `node_config`. Disk shrink and
unsupported capacity/topology changes remain server errors. CPU and memory apply
per node; replicated deployments have both metadata and data groups, so topology
changes materially affect the estimate. Inference capacity is managed separately.

Both commands accept `--request -` for one bounded JSON object on stdin and
`--json` for machine-readable output. They preserve the normal login, profile,
organization selection, management-token scopes, and authorization controls.
Provider credentials, source data, and Antfly table keys do not belong in these
request files.
