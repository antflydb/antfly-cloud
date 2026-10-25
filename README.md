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


See [managed instance capacity](docs/instance-capacity.md) for CLI creation, live credit estimates, and resizing.

For enterprise SSO, `antfly-cloud login --email you@your-company.com` pre-fills
account discovery in the browser. Your identity provider completes sign-in,
then you approve the CLI device request. If SSO reports
`SSO_MFA_POLICY_REQUIRED`, an organization admin must delegate authentication
in Settings > Identity, verify the provider, and enable it again. This delegates
MFA policy to your identity provider. The CLI cannot override that decision.
