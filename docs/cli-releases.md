# CLI binaries and releases

The CLI ships four `.tar.gz` archives containing `antfly-cloud` (built with
`CGO_ENABLED=0`), README, license, and shell completions:

| Platform | Archive suffix |
| --- | --- |
| macOS Intel | `Darwin_x86_64.tar.gz` |
| macOS Apple Silicon | `Darwin_arm64.tar.gz` |
| Linux x86-64 | `Linux_x86_64.tar.gz` |
| Linux ARM64 | `Linux_arm64.tar.gz` |

Windows, 32-bit systems, and ARMv7 are not currently built.

## Discover and install without Homebrew

Find `v*` tags in the [public repository tags](https://github.com/antflydb/antfly-cloud/tags),
or query them with GitHub CLI:

```sh
gh api --paginate repos/antflydb/antfly-cloud/tags \
  --jq '.[] | select(.name | startswith("v")) | .name'
```

A tag identifies a release attempt, not a successful upload. Check its
[CLI Release workflow](https://github.com/antflydb/antfly-cloud/actions/workflows/go-cli-release.yml)
and fetch its checksums before installing. GitHub Releases/assets and a mutable
`latest` alias are not published by this workflow.

Choose a published version explicitly, including `-rc.N` for a release candidate.
Tag `v1.2.3` maps to directory `v1.2.3` and archive version `1.2.3`.
The example URLs below become available only after that version is published.

```sh
set -eu
version=1.2.3 # replace with a published version
case "$(uname -s)" in
  Darwin) os=Darwin ;;
  Linux) os=Linux ;;
  *) echo 'Unsupported operating system' >&2; exit 1 ;;
esac
case "$(uname -m)" in
  x86_64|amd64) arch=x86_64 ;;
  arm64|aarch64) arch=arm64 ;;
  *) echo 'Unsupported architecture' >&2; exit 1 ;;
esac
archive="antfly-cloud_${version}_${os}_${arch}.tar.gz"
base="https://releases.antfly.io/antfly-cloud/v${version}"
tmp=$(mktemp -d)
(
  set -eu
  cd "$tmp"
  curl -fLO "$base/$archive"
  curl -fLO "$base/checksums.txt"
  awk -v file="$archive" '$2 == file { print }' checksums.txt > selected-checksum.txt
  test -s selected-checksum.txt
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum -c selected-checksum.txt
  else
    shasum -a 256 -c selected-checksum.txt
  fi
  tar -xzf "$archive"
  mkdir -p "$HOME/.local/bin"
  install -m 755 antfly-cloud "$HOME/.local/bin/antfly-cloud"
)
rm -rf "$tmp"
```

Ensure `$HOME/.local/bin` is on `PATH`, then run `antfly-cloud version`.
Checksums detect corrupt downloads; they are not an independent signature.
Each version directory also contains GoReleaser `metadata.json`. The local
`artifacts.json` manifest is generated after publication and is not uploaded.

## Maintainer publishing

Merge client changes and release configuration before tagging the intended commit.
Pushing `vX.Y.Z` or `vX.Y.Z-rc.N` triggers the CLI release workflow;
merging a PR or creating a GitHub release alone does not trigger it. The workflow
requires the tagged commit to belong to `main`, tests the CLI, and builds all
four targets without publishing credentials. Review the tag, commit, payload
manifest SHA256, and downloadable release artifact in the run summary. One of
the five release reviewers must approve publication through the protected
`antfly-cloud-cli-release` environment; the triggering user cannot self-approve.
Emergency administrator bypass is available. This applies to RC and stable tags.

After approval, the publisher rechecks the source and all payload checksums, then
uploads those exact files to R2 bucket `antfly-releases` under
`antfly-cloud/vX.Y.Z/`, including any prerelease suffix. It does not rebuild the
approved files. The environment permits only `v*` tags.

Configure `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`, `AWS_ENDPOINT_URL`, and
`HOMEBREW_APP_PRIVATE_KEY` only in the protected `antfly-cloud-cli-release`
environment. Cloud must not retain repository copies or access to the org App
private-key secret. Use the R2 account endpoint,
`https://<account-id>.r2.cloudflarestorage.com`, without a bucket suffix. R2 uses
region `auto` and path-style addressing. Existing public routing for
`releases.antfly.io` must serve this bucket/prefix; credentials alone do not
configure public access. Google publishing credentials are no longer needed.

Homebrew uses a short-lived installation token from the organization-owned
Homebrew publisher GitHub App to update `antflydb/homebrew-taps` for stable
releases. Configure the App credentials as described in the
[repository README](../README.md#release-approval-and-publishing-credentials).
The workflow mints the token only for stable releases after approval and passes
it to the publisher as `HOMEBREW_TAP_GITHUB_TOKEN`. Release candidates such as
`vX.Y.Z-rc.1` publish binaries and checksums while skipping the stable tap update.
Use the prerelease version with the direct installation instructions above.

For stable tags, the publisher uploads blobs before updating Homebrew. A denied tap
update fails the job visibly and does not roll back uploaded objects. Verify
CDN checksums and all four archives before treating a failed stable run as usable.
A failed job does not mean nothing was published; inspect the failed step before
retrying. Matching existing immutable objects are safe to retry; different
content at an existing version is rejected and requires a new version. Rerun
all jobs while the prepared artifact is retained if rebuilding is necessary.
A green RC run also requires public download verification: private
upload success does not prove public CDN availability.

Before announcing a release, download `checksums.txt` and all four archives from
the public CDN, verify every SHA256, and smoke-test `antfly-cloud version` on
supported platforms. Preparing this configuration does not itself publish a release.
