"""Exercise CI provisioning with real archive verification and no host mutations."""

import hashlib
import io
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tarfile
import tempfile
import unittest


SCRIPT = Path(__file__).resolve().parents[1] / "prepare_legacy_snapshot.sh"


class SnapshotPrerequisiteTests(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory()
        self.addCleanup(self.temporary.cleanup)
        self.root = Path(self.temporary.name)
        self.repo = self.root / "repo"
        helper = self.repo / "scripts/github_runner/prepare_legacy_snapshot.sh"
        helper.parent.mkdir(parents=True)
        shutil.copyfile(SCRIPT, helper)
        self.helper = helper
        self.bin = self.root / "bin"
        self.bin.mkdir()
        for command in ("dirname", "mkdir", "sha256sum", "mktemp", "tar",
                        "gzip", "mv", "rmdir", "rm"):
            (self.bin / command).symlink_to(shutil.which(command))
        (self.bin / "python3").symlink_to(sys.executable)
        self.calls = self.root / "calls.jsonl"
        self.archive = self.root / "go.tar.gz"
        self.go_program = b'#!/bin/sh\nprintf "go version go1.26.11 linux/amd64\\n"\n'
        with tarfile.open(self.archive, "w:gz") as archive:
            info = tarfile.TarInfo("go/bin/go")
            info.mode = 0o755
            info.size = len(self.go_program)
            archive.addfile(info, io.BytesIO(self.go_program))
        manifest = self.repo / "configs/setup/firecracker-stack.json"
        manifest.parent.mkdir(parents=True)
        manifest.write_text(json.dumps({"inputs": {"go": {
            "version": "1.26.11", "archive": "go1.26.11.linux-amd64.tar.gz",
            "sha256": hashlib.sha256(self.archive.read_bytes()).hexdigest(),
        }}}))
        self.env = dict(os.environ, PATH=str(self.bin), STACK_DIR=str(self.root / "stack"),
                        GITHUB_PATH=str(self.root / "github-path"),
                        MOCK_CALLS=str(self.calls), MOCK_ARCHIVE=str(self.archive))
        self.fake("uname", 'print("x86_64")')
        self.fake("docker", '''
if os.environ.get("FAIL_DOCKER") == sys.argv[1]:
    raise SystemExit("unusable Docker " + sys.argv[1])
print("existing Docker CE " + sys.argv[1])
''')
        self.fake("sudo", 'print("mock apt prerequisite transaction")')
        self.fake("dpkg-query", 'print("mock installed package inventory")')
        self.fake("route", 'print("mock route")')
        self.fake("curl", '''
destination = Path(sys.argv[sys.argv.index("--output") + 1])
destination.write_bytes(b"bad archive" if os.environ.get("CORRUPT_ARCHIVE") else Path(os.environ["MOCK_ARCHIVE"]).read_bytes())
''')

    def fake(self, name, body):
        path = self.bin / name
        path.write_text(f"#!{sys.executable}\n" + '''import json, os, sys
from pathlib import Path
with open(os.environ["MOCK_CALLS"], "a") as output:
    output.write(json.dumps([Path(sys.argv[0]).name, *sys.argv[1:]]) + "\\n")
''' + body + "\n")
        path.chmod(0o755)

    def run_helper(self):
        return subprocess.run(["/bin/bash", str(self.helper)], env=self.env,
                              text=True, capture_output=True, check=False)

    def commands(self):
        return [json.loads(line) for line in self.calls.read_text().splitlines()]

    def test_preserves_effective_user_docker_and_installs_verified_go(self):
        result = self.run_helper()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        commands = self.commands()
        apt = next(c for c in commands if c[:6] == ["sudo", "-n", "env", "DEBIAN_FRONTEND=noninteractive", "apt-get", "install"])
        self.assertIn("--no-remove", apt)
        self.assertIn("net-tools", apt)
        self.assertIn("libseccomp-dev", apt)
        self.assertIn("debootstrap", apt)
        self.assertIn("dwarves", apt)
        self.assertFalse(any(package in apt for package in ("docker.io", "docker-buildx", "containerd", "containerd.io")))
        self.assertEqual([c for c in commands if c[0] == "docker"],
                         [["docker", "version"], ["docker", "buildx", "version"]] * 2)
        self.assertFalse(any("docker" in c for c in commands if c[0] == "sudo"))
        go = self.root / "stack/toolchains/go1.26.11/bin/go"
        self.assertEqual(go.read_bytes(), self.go_program)
        self.assertEqual((self.root / "github-path").read_text(), str(go.parent) + "\n")
        self.assertIn("go version go1.26.11 linux/amd64", result.stdout)

    def test_unusable_docker_or_buildx_fails_before_apt_or_download(self):
        for failure in ("version", "buildx"):
            with self.subTest(failure=failure):
                self.calls.unlink(missing_ok=True)
                self.env["FAIL_DOCKER"] = failure
                result = self.run_helper()
                self.assertNotEqual(result.returncode, 0)
                self.assertFalse(any(c[0] in ("sudo", "curl") for c in self.commands()))

    def test_corrupt_go_archive_cannot_be_installed(self):
        self.env["CORRUPT_ARCHIVE"] = "1"
        result = self.run_helper()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("FAILED", result.stdout)
        self.assertFalse((self.root / "stack/toolchains/go1.26.11").exists())
        self.assertFalse((self.root / "github-path").exists())

    def test_wrong_cached_go_fails_without_fallback_download(self):
        go = self.root / "stack/toolchains/go1.26.11/bin/go"
        go.parent.mkdir(parents=True)
        go.write_text('#!/bin/sh\nprintf "go version go1.25.0 linux/amd64\\n"\n')
        go.chmod(0o755)
        result = self.run_helper()
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse(any(c[0] == "curl" for c in self.commands()))
        self.assertFalse((self.root / "github-path").exists())

    def test_missing_route_fails_before_toolchain_install(self):
        (self.bin / "route").unlink()
        result = self.run_helper()
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse(any(c[0] == "curl" for c in self.commands()))


if __name__ == "__main__":
    unittest.main()
