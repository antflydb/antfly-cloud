# Antfly Cloud CLI

`antfly-cloud` is the public command-line and terminal UI client for Antfly Cloud. It
is intentionally separate from the private Colony backend runner named
`colony cloudaf`, which starts the Antfly Cloud API server.

## Install

The first supported install channel is Homebrew:

```sh
brew install antflydb/taps/antfly-cloud
```

Release archives are hosted at `https://releases.antfly.io/antfly-cloud/` and the
Homebrew formula lives in the public `antflydb/homebrew-taps` repository.
Colony source can remain private because the formula downloads prebuilt release
archives, not source code.

## Build from this repository

```sh
cd go
GOWORK=off go build -o ../bin/antfly-cloud ./cmd/antfly-cloud
```

From the Colony checkout, the submodule-backed build target is:

```sh
make build-antfly-cloud-cli
```

## Version and completions

```sh
antfly-cloud version
antfly-cloud completion zsh > "${fpath[1]}/_antfly-cloud"
antfly-cloud completion bash > antfly-cloud.bash
antfly-cloud completion fish > ~/.config/fish/completions/antfly-cloud.fish
```

## Configuration

The CLI loads config with Viper from `./.antfly/cloud/config.yaml`,
`./.antfly/cloud/config.json`, `~/.antfly/cloud/config.yaml`, `~/.antfly/cloud/config.json`, or
`ANTFLY_CLOUD_CONFIG`. When no config exists, default writes create
`~/.antfly/cloud/config.yaml` so other tools can consume the active Antfly Cloud profile.
A project-local `./.antfly/cloud/config.*` file overrides the global profile when
present, and `--config` can point to either a config file or a config directory:

```sh
antfly-cloud --config .local/antfly-cloud-cli/config.yaml status
```

Environment overrides are also supported: `ANTFLY_CLOUD_CONFIG`, `ANTFLY_CLOUD_TOKEN`,
`ANTFLY_CLOUD_API_URL`, `ANTFLY_CLOUD_ORG`, and `ANTFLY_CLOUD_INSTANCE`.

## Login

Log in with the Antfly Cloud device flow:

```sh
antfly-cloud login
```

The CLI opens the verification URL when possible and also prints the URL and
one-time code. It does not start a localhost callback server. The CLI stores the
OIDC ID token and refresh token, then refreshes automatically when needed. For
manual token/dev flows, `antfly-cloud login --token ...` remains available.

## Common commands

```sh
antfly-cloud status
antfly-cloud instances
antfly-cloud usage
antfly-cloud instance use prod
antfly-cloud context --json
eval "$(antfly-cloud env)"
```

`antfly-cloud env` exports data-plane connection settings for the Antfly CLI.
Antfly Cloud management API keys (`antfly_cloud_*`) cannot be used as Antfly data-plane
tokens; use device login for that flow.

## Local Colony stacks

For local Colony stacks, run `make antfly-cloud-local-setup` once from the Colony
repository root to seed local dev data and write `.local/antfly-cloud-cli/config.json`,
then run commands through the same binary:

```sh
make antfly-cloud-local ARGS="instances"
bin/antfly-cloud --config .local/antfly-cloud-cli/config.json usage
```

The CLI is public/untrusted software. Dev, local, and production access must be
enforced by the Antfly Cloud API through Antfly authentication, organization
membership, scoped `antfly_cloud_*` management keys, and environment-specific RBAC —
not by hiding CLI code or flags.
