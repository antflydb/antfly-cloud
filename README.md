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

## Install the CLI

Prebuilt macOS and Linux binaries, checksum verification, and release discovery
are documented in [CLI releases](docs/cli-releases.md). Homebrew is optional.

## Development

Use Node.js 26.13.0 LTS (`.node-version`), npm 12.2.1, pnpm 12.16.1
(`ts/package.json`), and Go 1.27.3 (`go/go.mod`). Node 26 does not bundle
Corepack; install it before enabling the pinned pnpm version:

```sh
npm install --global npm@12.2.1 corepack@0.36.0
corepack enable
corepack prepare pnpm@12.16.1 --activate
```

The TypeScript SDK supports Node.js 20.19+ and browsers. Development and CI use
the pinned LTS version. TypeScript stays on 6.0.x because the declaration build
uses its JavaScript compiler API, which TypeScript 7 does not provide.

Dependabot checks Actions, Go modules, and the pnpm workspace weekly. Minor and
patch dependency updates are grouped; major updates remain separate for review.
TypeScript major updates are held until the declaration tooling supports them.
Keep runtime files, package-manager pins, and release workflows synchronized when
upgrading the toolchain. There is no Python project in this repository.

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

See [connected sources](docs/connected-sources.md) for browser authorization, resource selection, attachment previews and private sync.

## Release approval and publishing credentials

The CLI release workflow tests and packages `v*` tags whose commits belong to
`main`, then records the exact payload and its SHA256 manifest for review.
Both release candidates and stable releases require approval through the
`antfly-cloud-cli-release` environment. One of the same five Antfly release
reviewers must approve; the person who triggered the run cannot self-approve.
Emergency administrator bypass remains enabled, matching Antfly.

Store `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`, `AWS_ENDPOINT_URL`, and
`HOMEBREW_APP_PRIVATE_KEY` as secrets in that protected environment. Remove
Cloud's repository copies and access to the organization private-key secret so
other jobs cannot retrieve the publishing credentials without approval. The
App client ID remains the shared organization variable `HOMEBREW_APP_CLIENT_ID`.
The App is installed only on `antflydb/homebrew-taps` with Contents read/write
access. Stable releases mint a short-lived tap-only token after approval;
release candidates skip the stable formula update and do not mint that token.

The publisher verifies the original build artifact ID, manifest digest, tag,
commit, and every file before uploading the prepared files. It does not rebuild
after approval, and refuses to overwrite a different versioned R2 object.
Rotate the App key by updating its organization secret for Antfly and its
protected environment secret for Cloud before deleting the old App key.
See [CLI releases](docs/cli-releases.md) for release and recovery steps.
