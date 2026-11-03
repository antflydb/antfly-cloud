#!/usr/bin/env python3
"""Prepare and publish the exact Cloud CLI payload approved by reviewers."""

import argparse
import base64
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import urllib.error
import urllib.request

NUMBER = r"(?:0|[1-9][0-9]*)"
PRERELEASE = r"(?:0|[1-9][0-9]*|[0-9]*[A-Za-z-][0-9A-Za-z-]*)"
TAG = re.compile(
    rf"v{NUMBER}\.{NUMBER}\.{NUMBER}(?:-{PRERELEASE}(?:\.{PRERELEASE})*)?(?:\+[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?"
)
PLATFORMS = ("Darwin_arm64", "Darwin_x86_64", "Linux_arm64", "Linux_x86_64")
FORMULA = "Formula/antfly-cloud.rb"


def identity(tag, commit):
    if not TAG.fullmatch(tag) or not re.fullmatch(r"[0-9a-f]{40}", commit):
        raise ValueError("Expected a v-prefixed SemVer tag and full commit SHA")
    return tag[1:]


def validate_ref(tag, commit):
    identity(tag, commit)
    subprocess.run(
        [
            "git",
            "fetch",
            "--no-tags",
            "origin",
            "+refs/heads/main:refs/remotes/origin/main",
        ],
        check=True,
        capture_output=True,
    )
    for ref in ("HEAD", f"refs/tags/{tag}^{{commit}}"):
        actual = subprocess.check_output(
            ["git", "rev-parse", "--verify", ref], text=True
        ).strip()
        if actual != commit:
            raise ValueError("Tag, checked-out source, and workflow commit must match")
    result = subprocess.run(
        ["git", "merge-base", "--is-ancestor", commit, "refs/remotes/origin/main"]
    )
    if result.returncode:
        raise ValueError("Release commit must belong to main")


def sha256(path):
    with path.open("rb") as source:
        return hashlib.file_digest(source, "sha256").hexdigest()


def filenames(tag, commit):
    version = identity(tag, commit)
    archives = [f"antfly-cloud_{version}_{platform}.tar.gz" for platform in PLATFORMS]
    return archives + ["metadata.json", "checksums.txt", FORMULA]


def verify_contents(root, tag, commit):
    expected = filenames(tag, commit)
    for name in expected:
        path = root / name
        if not path.is_file() or path.is_symlink() or path.parent.is_symlink():
            raise ValueError("Missing or unsafe release file: " + name)
    metadata = json.loads((root / "metadata.json").read_text())
    if any(
        metadata.get(key) != value
        for key, value in {
            "project_name": "antfly-cloud",
            "tag": tag,
            "version": tag[1:],
            "commit": commit,
        }.items()
    ):
        raise ValueError("Release metadata does not match the approved tag and commit")
    checksums = {}
    for line in (root / "checksums.txt").read_text().splitlines():
        fields = line.split()
        if (
            len(fields) != 2
            or fields[1] in checksums
            or not re.fullmatch(r"[0-9a-f]{64}", fields[0])
        ):
            raise ValueError("Malformed or duplicate checksum entry")
        checksums[fields[1]] = fields[0]
    if set(checksums) != set(expected[:4]):
        raise ValueError("Checksums must describe exactly the four expected archives")
    if any(sha256(root / name) != digest for name, digest in checksums.items()):
        raise ValueError("Archive checksum mismatch")
    return expected


def prepare(dist, output, tag, commit):
    identity(tag, commit)
    if output.exists():
        raise ValueError("Payload output directory already exists")
    output.mkdir(parents=True)
    for name in filenames(tag, commit):
        source = dist / ("homebrew/" + name if name == FORMULA else name)
        if source.is_symlink() or not source.is_file():
            raise ValueError("Missing or unsafe build output: " + name)
        dest = output / name
        dest.parent.mkdir(parents=True, exist_ok=True)
        shutil.copyfile(source, dest)
    names = verify_contents(output, tag, commit)
    manifest = {
        "schema": 1,
        "tag": tag,
        "commit": commit,
        "files": {name: sha256(output / name) for name in names},
    }
    (output / "manifest.json").write_text(
        json.dumps(manifest, sort_keys=True, indent=2) + "\n"
    )
    return sha256(output / "manifest.json")


def verify_payload(root, tag, commit, digest):
    identity(tag, commit)
    manifest_path = root / "manifest.json"
    if (
        manifest_path.is_symlink()
        or not re.fullmatch(r"[0-9a-f]{64}", digest)
        or sha256(manifest_path) != digest
    ):
        raise ValueError("Payload manifest differs from the build job output")
    manifest = json.loads(manifest_path.read_text())
    expected = filenames(tag, commit)
    if (
        manifest.get("schema") != 1
        or manifest.get("tag") != tag
        or manifest.get("commit") != commit
        or set(manifest.get("files", {})) != set(expected)
    ):
        raise ValueError("Payload identity or file set mismatch")
    if {
        str(path.relative_to(root))
        for path in root.rglob("*")
        if path.is_file() or path.is_symlink()
    } != set(expected) | {"manifest.json"}:
        raise ValueError("Payload contains unexpected files")
    verify_contents(root, tag, commit)
    if any(sha256(root / name) != manifest["files"][name] for name in expected):
        raise ValueError("Payload file differs from approved manifest")
    return expected


def upload_objects(root, names, tag, client, client_error, bucket="antfly-releases"):
    for name in names:
        if name == FORMULA:
            continue
        key = f"antfly-cloud/{tag}/{name}"
        digest = sha256(root / name)
        try:
            with (root / name).open("rb") as source:
                client.put_object(
                    Bucket=bucket,
                    Key=key,
                    Body=source,
                    IfNoneMatch="*",
                    Metadata={"sha256": digest},
                    ContentType="application/gzip"
                    if name.endswith(".gz")
                    else (
                        "application/json"
                        if name.endswith(".json")
                        else "text/plain; charset=utf-8"
                    ),
                    ContentDisposition=f"attachment;filename={name}",
                )
        except client_error as exc:
            status = exc.response.get("ResponseMetadata", {}).get("HTTPStatusCode")
            code = str(exc.response.get("Error", {}).get("Code"))
            if status not in (409, 412) and code not in (
                "409",
                "412",
                "ConditionalRequestConflict",
                "PreconditionFailed",
            ):
                raise
            remote = client.get_object(Bucket=bucket, Key=key)["Body"]
            try:
                remote_digest = hashlib.file_digest(remote, "sha256").hexdigest()
            finally:
                remote.close()
            if remote_digest != digest:
                raise ValueError(
                    "Refusing to overwrite a different immutable object: " + key
                ) from None
        print("Verified publication: " + key)


def github_request(path, token, method="GET", payload=None):
    request = urllib.request.Request(
        "https://api.github.com" + path,
        method=method,
        data=None if payload is None else json.dumps(payload).encode(),
        headers={
            "Authorization": "Bearer " + token,
            "Accept": "application/vnd.github+json",
            "User-Agent": "antfly-cloud-release",
        },
    )
    with urllib.request.urlopen(request, timeout=30) as response:
        return json.load(response)


def publish_formula(root, tag, token, request=github_request):
    if "-" in tag.split("+", 1)[0]:
        print("Release candidate: stable Homebrew formula update skipped")
        return
    path = "/repos/antflydb/homebrew-taps/contents/" + FORMULA
    formula = (root / FORMULA).read_bytes()
    for attempt in range(3):
        sha = None
        try:
            current = request(path + "?ref=main", token)
            if base64.b64decode(current["content"]) == formula:
                print("Homebrew formula already matches")
                return
            sha = current["sha"]
        except urllib.error.HTTPError as exc:
            if exc.code != 404:
                raise
        payload = {
            "message": "Update antfly-cloud to " + tag,
            "branch": "main",
            "content": base64.b64encode(formula).decode(),
        }
        if sha:
            payload["sha"] = sha
        try:
            request(path, token, "PUT", payload)
            print("Updated stable Homebrew formula to " + tag)
            return
        except urllib.error.HTTPError as exc:
            if exc.code not in (409, 422) or attempt == 2:
                raise


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "operation", choices=("validate-ref", "prepare", "verify", "publish")
    )
    parser.add_argument("--tag", required=True)
    parser.add_argument("--commit", required=True)
    parser.add_argument("--dist", type=Path, default=Path("go/dist"))
    parser.add_argument("--payload", type=Path, default=Path("release-payload"))
    parser.add_argument("--manifest-sha256")
    args = parser.parse_args()
    if args.operation == "validate-ref":
        validate_ref(args.tag, args.commit)
    elif args.operation == "prepare":
        print(prepare(args.dist, args.payload, args.tag, args.commit))
    else:
        names = verify_payload(
            args.payload, args.tag, args.commit, args.manifest_sha256 or ""
        )
        if args.operation == "verify":
            print("Payload tag, commit, manifest, and all file checksums verified")
            return
        import boto3
        from botocore.config import Config
        from botocore.exceptions import ClientError

        client = boto3.client(
            "s3",
            endpoint_url=os.environ["AWS_ENDPOINT_URL"],
            region_name="auto",
            aws_access_key_id=os.environ["AWS_ACCESS_KEY_ID"],
            aws_secret_access_key=os.environ["AWS_SECRET_ACCESS_KEY"],
            config=Config(
                signature_version="s3v4",
                s3={"addressing_style": "path"},
                request_checksum_calculation="when_required",
                response_checksum_validation="when_required",
            ),
        )
        upload_objects(args.payload, names, args.tag, client, ClientError)
        publish_formula(args.payload, args.tag, os.environ["HOMEBREW_TAP_GITHUB_TOKEN"])


if __name__ == "__main__":
    main()
