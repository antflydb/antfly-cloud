import base64
from contextlib import chdir
import io
import json
from pathlib import Path
import subprocess
import tempfile
import unittest

import cloud_cli as release

COMMIT = "a" * 40
TAG = "v1.2.3-rc.1"


class FakeError(Exception):
    response = {"ResponseMetadata": {"HTTPStatusCode": 412}}


class FakeS3:
    def __init__(self):
        self.objects = {}

    def put_object(self, **kwargs):
        assert kwargs["IfNoneMatch"] == "*"
        key = kwargs["Key"]
        if key in self.objects:
            raise FakeError()
        self.objects[key] = kwargs["Body"].read()

    def get_object(self, **kwargs):
        return {"Body": io.BytesIO(self.objects[kwargs["Key"]])}


class ReleaseTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)
        self.dist = self.root / "dist"
        self.dist.mkdir()
        self.payload = self.root / "payload"
        names = release.filenames(TAG, COMMIT)
        for name in names[:4]:
            (self.dist / name).write_bytes(name.encode())
        (self.dist / "checksums.txt").write_text(
            "".join(
                f"{release.sha256(self.dist / name)}  {name}\n" for name in names[:4]
            )
        )
        (self.dist / "metadata.json").write_text(
            json.dumps(
                {
                    "project_name": "antfly-cloud",
                    "tag": TAG,
                    "version": TAG[1:],
                    "commit": COMMIT,
                }
            )
        )
        formula = self.dist / "homebrew" / release.FORMULA
        formula.parent.mkdir(parents=True)
        formula.write_text("class AntflyCloud < Formula\nend\n")
        self.digest = release.prepare(self.dist, self.payload, TAG, COMMIT)

    def test_payload_is_bound_to_tag_commit_and_expected_manifest(self):
        names = release.verify_payload(self.payload, TAG, COMMIT, self.digest)
        self.assertEqual(len(names), 7)
        for tag, commit, digest in [
            ("v1.2.3", COMMIT, self.digest),
            (TAG, "b" * 40, self.digest),
            (TAG, COMMIT, "0" * 64),
        ]:
            with (
                self.subTest(tag=tag, commit=commit, digest=digest),
                self.assertRaises(ValueError),
            ):
                release.verify_payload(self.payload, tag, commit, digest)

    def test_rejects_modified_formula_and_unexpected_files(self):
        formula = self.payload / release.FORMULA
        original = formula.read_bytes()
        formula.write_text("tampered")
        with self.assertRaisesRegex(ValueError, "differs from approved"):
            release.verify_payload(self.payload, TAG, COMMIT, self.digest)
        formula.write_bytes(original)
        (self.payload / "artifacts.json").write_text("{}")
        with self.assertRaisesRegex(ValueError, "unexpected files"):
            release.verify_payload(self.payload, TAG, COMMIT, self.digest)

    def test_rejects_archive_corruption_and_symlinks(self):
        archive = self.payload / release.filenames(TAG, COMMIT)[0]
        archive.write_text("corrupt")
        with self.assertRaisesRegex(ValueError, "checksum mismatch"):
            release.verify_payload(self.payload, TAG, COMMIT, self.digest)
        archive.unlink()
        archive.symlink_to(self.dist / archive.name)
        with self.assertRaisesRegex(ValueError, "unsafe release file"):
            release.verify_payload(self.payload, TAG, COMMIT, self.digest)

    def test_rejects_duplicate_checksums_during_prepare(self):
        checksums = self.dist / "checksums.txt"
        checksums.write_text(
            checksums.read_text() + checksums.read_text().splitlines()[0] + "\n"
        )
        with self.assertRaisesRegex(ValueError, "duplicate checksum"):
            release.prepare(self.dist, self.root / "other-payload", TAG, COMMIT)

    def test_uploads_only_public_files_and_retries_without_overwrite(self):
        client = FakeS3()
        names = release.filenames(TAG, COMMIT)
        release.upload_objects(self.payload, names, TAG, client, FakeError)
        self.assertEqual(len(client.objects), 6)
        self.assertTrue(
            all(name.startswith("antfly-cloud/" + TAG + "/") for name in client.objects)
        )
        release.upload_objects(self.payload, names, TAG, client, FakeError)
        key = next(iter(client.objects))
        client.objects[key] = b"different public object"
        with self.assertRaisesRegex(ValueError, "Refusing to overwrite"):
            release.upload_objects(self.payload, names, TAG, client, FakeError)

    def test_prerelease_never_updates_stable_formula(self):
        def forbidden(*args):
            self.fail("RC attempted a GitHub API request")

        release.publish_formula(self.payload, TAG, "test-token", forbidden)

    def test_stable_formula_update_uses_expected_repository_and_blob_sha(self):
        calls = []

        def request(path, token, method="GET", payload=None):
            calls.append((path, method, payload))
            if method == "GET":
                return {
                    "sha": "old-blob",
                    "content": base64.b64encode(b"old formula").decode(),
                }
            return {}

        release.publish_formula(self.payload, "v1.2.3", "test-token", request)
        path, method, payload = calls[1]
        self.assertEqual(
            path, "/repos/antflydb/homebrew-taps/contents/Formula/antfly-cloud.rb"
        )
        self.assertEqual(method, "PUT")
        self.assertEqual(payload["branch"], "main")
        self.assertEqual(payload["sha"], "old-blob")
        self.assertEqual(
            base64.b64decode(payload["content"]),
            (self.payload / release.FORMULA).read_bytes(),
        )

    def test_semver_validation(self):
        for tag in ["v0.1.0", "v1.2.3-rc.1", "v1.2.3-rc.1+build.9"]:
            release.identity(tag, COMMIT)
        for tag in [
            "client-v1.2.3",
            "1.2.3",
            "v01.2.3",
            "v1.2.3-01",
            "v1.2.3-rc..1",
            "v../main",
        ]:
            with self.subTest(tag=tag), self.assertRaises(ValueError):
                release.identity(tag, COMMIT)


class RefTests(unittest.TestCase):
    def test_tag_must_resolve_to_checked_out_commit_on_main(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            repo = root / "source"
            repo.mkdir()

            def git(*args):
                return subprocess.check_output(
                    ["git", *args], cwd=repo, stderr=subprocess.DEVNULL, text=True
                ).strip()

            git("init", "--initial-branch=main")
            git("config", "user.name", "Release test")
            git("config", "user.email", "release-test@example.com")
            (repo / "source.txt").write_text("trusted")
            git("add", ".")
            git("commit", "-m", "trusted main")
            trusted = git("rev-parse", "HEAD")
            git("tag", "v1.2.3")
            git("clone", "--bare", str(repo), str(root / "origin.git"))
            git("remote", "add", "origin", str(root / "origin.git"))
            with chdir(repo):
                release.validate_ref("v1.2.3", trusted)
            git("switch", "-c", "unreviewed")
            (repo / "source.txt").write_text("untrusted")
            git("commit", "-am", "not on main")
            untrusted = git("rev-parse", "HEAD")
            git("tag", "v1.2.4")
            with chdir(repo):
                with self.assertRaisesRegex(ValueError, "must belong to main"):
                    release.validate_ref("v1.2.4", untrusted)
                with self.assertRaisesRegex(ValueError, "must match"):
                    release.validate_ref("v1.2.3", trusted)


if __name__ == "__main__":
    unittest.main()
