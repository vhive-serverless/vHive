import copy
import importlib.util
import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest.mock import patch


SPEC = importlib.util.spec_from_file_location("firecracker_stack", Path(__file__).parents[1] / "firecracker_stack.py")
STACK = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(STACK)


class FirecrackerStackTests(unittest.TestCase):
    def setUp(self):
        self.directory = tempfile.TemporaryDirectory()
        self.addCleanup(self.directory.cleanup)
        self.root = Path(self.directory.name)
        self.manifest = STACK.load_manifest(STACK.DEFAULT_MANIFEST)

    def write(self, path, content):
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(content)

    def test_pending_pin_cannot_start_installation(self):
        manifest = copy.deepcopy(self.manifest)
        manifest["runtime"]["ready"] = False
        path = self.root / "manifest.json"
        path.write_text(json.dumps(manifest))
        with patch.dict(os.environ, {}, clear=True), patch("sys.argv", ["stack", "install", "--manifest", str(path)]), patch.object(STACK, "run") as run:
            with self.assertRaisesRegex(ValueError, "runtime pin is pending"):
                STACK.main()
            run.assert_not_called()

    def test_release_and_artifact_paths_are_validated(self):
        for edit in (lambda value: value["artifacts"]["kernel"].update(installed="../../outside"),
                     lambda value: value["inputs"]["firecracker"].update(commit="f" * 40)):
            manifest = copy.deepcopy(self.manifest)
            edit(manifest)
            path = self.root / "manifest.json"
            path.write_text(json.dumps(manifest))
            with self.assertRaises(ValueError):
                STACK.load_manifest(path)

    def test_source_lock_must_match_all_selected_inputs(self):
        source = self.root / "source"
        self.write(source / "docs/modernization/versions.json", json.dumps({"selected_inputs": self.manifest["inputs"]}))
        subprocess.run(["git", "init", "--quiet", str(source)], check=True)
        subprocess.run(["git", "add", "."], cwd=source, check=True)
        subprocess.run(["git", "-c", "user.name=test", "-c", "user.email=test@example.invalid", "commit", "--quiet", "-m", "fixture"], cwd=source, check=True)
        revision = STACK.run("git", "rev-parse", "HEAD", cwd=source, capture=True).strip()
        STACK.check_checkout(source, revision, self.manifest)
        changed = copy.deepcopy(self.manifest)
        changed["inputs"]["runc"]["version"] = "v0.0.0"
        with self.assertRaisesRegex(ValueError, "runc.version"):
            STACK.check_checkout(source, revision, changed)
        with self.assertRaisesRegex(ValueError, "expected"):
            STACK.check_checkout(source, "f" * 40, self.manifest)

    def install_fixture(self):
        source, prefix = self.root / "source", self.root / "install"
        revision = "1" * 40
        self.write(source / "docs/modernization/versions.json", json.dumps({"selected_inputs": self.manifest["inputs"]}))
        self.write(source / "bin/receipts/build/runtime-revision", revision + "\n")
        self.write(source / "bin/receipts/build/binaries.sha256", "fixture build receipt\n")
        self.write(source / "agent/agent", "co-versioned agent\n")
        for name, artifact in self.manifest["artifacts"].items():
            self.write(source / artifact["source"], name)
            self.write(prefix / artifact["installed"], name)
        receipt = STACK.make_receipt(source, prefix, revision, self.manifest)
        self.write(prefix / "vhive-stack-receipt.json", json.dumps(receipt))
        return source, prefix, revision

    def test_receipt_covers_runtime_guest_and_snapshotter(self):
        _, prefix, revision = self.install_fixture()
        receipt = STACK.verify_install(prefix, revision, self.manifest)
        self.assertIn("share/runtime/default-rootfs.img", receipt["installed_sha256"])
        self.assertIn("share/runtime/default-vmlinux.bin", receipt["installed_sha256"])
        self.assertIn("bin/demux-snapshotter", receipt["installed_sha256"])
        self.assertIn("bin/runc", receipt["installed_sha256"])
        self.assertEqual(64, len(receipt["guest_agent_sha256"]))

    def test_stale_build_cannot_produce_install_receipt(self):
        source, prefix, revision = self.install_fixture()
        self.write(source / "bin/receipts/build/runtime-revision", "2" * 40)
        with self.assertRaisesRegex(ValueError, "different revision"):
            STACK.make_receipt(source, prefix, revision, self.manifest)

    def test_tampered_artifact_is_rejected_before_activation(self):
        _, prefix, revision = self.install_fixture()
        self.write(prefix / "bin/demux-snapshotter", "stale unrelated binary")
        with patch.object(STACK, "run") as run:
            with self.assertRaisesRegex(ValueError, "differs from receipt"):
                STACK.activate(prefix, revision, self.manifest, "pool")
            run.assert_not_called()

    def test_incomplete_receipt_cannot_omit_artifacts(self):
        _, prefix, revision = self.install_fixture()
        path = prefix / "vhive-stack-receipt.json"
        receipt = json.loads(path.read_text())
        del receipt["installed_sha256"]["bin/runc"]
        path.write_text(json.dumps(receipt))
        with self.assertRaisesRegex(ValueError, "exactly the manifest artifacts"):
            STACK.verify_install(prefix, revision, self.manifest)


if __name__ == "__main__":
    unittest.main()
