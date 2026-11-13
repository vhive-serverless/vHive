import importlib.util
import json
import os
from pathlib import Path
import subprocess
import tempfile
from types import SimpleNamespace
import unittest
from unittest.mock import patch

SPEC = importlib.util.spec_from_file_location("legacy_pool", Path(__file__).parents[1] / "legacy_snapshot_pool.py")
POOL = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(POOL)


class FakeSystem(POOL.System):
    def __init__(self, source, pool):
        self.source, self.pool = source, pool
        self.exists = False
        self.attached = []
        self.commands = []
        self.fail_create = False
        self.capacity = POOL.DATA_BYTES
        self.foreign = ""

    def mappings(self):
        return {self.pool} if self.exists else set()

    def loops(self):
        return list(self.attached)

    def device(self, path):
        return {"/dev/loop10": "7:10", "/dev/loop11": "7:11", "/dev/mapper/" + self.pool: "252:42"}[path]

    def table(self):
        return f"0 {self.capacity // 512} thin-pool 7:11 7:10 128 32768 1 skip_block_zeroing\n"

    def run(self, *args):
        self.commands.append(args)
        if args[0] == "bash":
            if args[2] == "create":
                directory = self.source / "tmp/devmapper" / self.pool
                self.attached = [{"name": "/dev/loop10", "back-file": str(directory / "data")}]
                if self.fail_create:
                    raise subprocess.CalledProcessError(1, args, output="simulated partial loop attachment")
                self.attached.append({"name": "/dev/loop11", "back-file": str(directory / "metadata")})
                self.exists = True
            else:
                assert args[2] == "remove"
                self.exists = False
                self.attached = []
                for name in ["data", "metadata"]:
                    (self.source / "tmp/devmapper" / self.pool / name).unlink(missing_ok=True)
            return ""
        if args[:4] == ("sudo", "-n", "dmsetup", "table"):
            return (self.pool + ": " + self.table() if self.exists else "") + self.foreign if args[-1] == "--noheadings" else self.table()
        if args[:4] == ("sudo", "-n", "dmsetup", "status"):
            return f"0 {self.capacity // 512} thin-pool 0 0/262144 0/{self.capacity // 65536} - rw discard_passdown queue_if_no_space -\n"
        if args[:4] == ("sudo", "-n", "losetup", "--detach"):
            self.attached = [x for x in self.attached if x["name"] != args[4]]
            return ""
        raise AssertionError("unexpected real-system command: " + repr(args))


class LegacyPoolTests(unittest.TestCase):
    def setUp(self):
        self.directory = tempfile.TemporaryDirectory()
        self.addCleanup(self.directory.cleanup)
        self.root = Path(self.directory.name)
        self.source = self.root / "runtime"
        (self.source / "tools").mkdir(parents=True)
        (self.source / "tools/thinpool.sh").write_text("fixture runtime helper\n")
        self.pool = "vhive-private-test"
        self.evidence = self.root / "evidence"
        self.system = FakeSystem(self.source, self.pool)
        self.free = patch.object(POOL.os, "statvfs", return_value=SimpleNamespace(f_bavail=44 * POOL.GIB, f_frsize=1))
        self.free.start()
        self.addCleanup(self.free.stop)

    def create(self):
        return POOL.create(self.source, self.pool, self.evidence, self.system)

    def test_actual_sparse_sizes_geometry_and_remove(self):
        owner = self.create()
        directory = Path(owner["directory"])
        self.assertEqual((directory / "data").stat().st_size, 40 * POOL.GIB)
        self.assertEqual((directory / "metadata").stat().st_size, POOL.GIB)
        self.assertEqual(owner["plan"]["base_image_bytes"], 10 * POOL.GIB)
        self.assertLess((directory / "data").stat().st_blocks * 512, POOL.GIB)
        self.assertEqual((directory / "data").stat().st_mode & 0o777, 0o600)
        observed = json.loads((self.evidence / "capacity-verified.json").read_text())
        self.assertEqual(observed["data_bytes"], 40 * POOL.GIB)
        self.assertEqual(observed["available_data_bytes"], 40 * POOL.GIB)
        POOL.remove(self.evidence / "ownership.json", self.system)
        self.assertFalse(self.system.exists)
        self.assertFalse(self.system.attached)
        self.assertFalse((directory / "data").exists())
        self.assertTrue(POOL.remove(self.evidence / "ownership.json", self.system)["removed"])

    def test_insufficient_host_space_refuses_before_backing_claim(self):
        with patch.object(POOL.os, "statvfs", return_value=SimpleNamespace(f_bavail=43 * POOL.GIB - 1, f_frsize=1)):
            with self.assertRaisesRegex(ValueError, "43 GiB"):
                self.create()
        self.assertFalse((self.source / "tmp").exists())
        self.assertFalse(self.system.commands)

    def test_profile_change_cannot_silently_shrink_disk(self):
        config = self.root / "config.toml"
        config.write_text('[plugins."io.containerd.snapshotter.v1.devmapper"]\nbase_image_size="1GB"\n')
        with self.assertRaisesRegex(ValueError, "10GB profile"):
            POOL.capacity_plan(config, 44 * POOL.GIB)

    def test_existing_mapper_is_never_reused(self):
        self.system.exists = True
        with self.assertRaisesRegex(ValueError, "already exists"):
            self.create()
        self.assertTrue(self.system.exists)
        self.assertFalse(self.system.commands)

    def test_existing_path_and_backing_are_never_resized(self):
        directory = self.source / "tmp/devmapper" / self.pool
        directory.mkdir(parents=True)
        (directory / "data").write_bytes(b"existing")
        with self.assertRaisesRegex(ValueError, "already exists"):
            self.create()
        self.assertEqual((directory / "data").read_bytes(), b"existing")

    def test_existing_loop_refuses_new_claim(self):
        path = self.source / "tmp/devmapper" / self.pool / "data"
        self.system.attached = [{"name": "/dev/loop10", "back-file": str(path) + " (deleted)"}]
        with self.assertRaisesRegex(ValueError, "associated loop"):
            self.create()
        self.assertEqual(len(self.system.attached), 1)

    def test_symlink_parent_cannot_redirect_creation(self):
        outside = self.root / "outside"
        outside.mkdir()
        (self.source / "tmp").symlink_to(outside, target_is_directory=True)
        with self.assertRaisesRegex(ValueError, "symlink"):
            self.create()
        self.assertEqual(list(outside.iterdir()), [])

    def test_partial_create_detaches_only_its_owned_loop(self):
        self.system.fail_create = True
        with self.assertRaises(subprocess.CalledProcessError):
            self.create()
        self.assertFalse(self.system.exists)
        self.assertFalse(self.system.attached)
        owner = json.loads((self.evidence / "ownership.json").read_text())
        self.assertTrue(owner["removed"])
        self.assertIn(("sudo", "-n", "losetup", "--detach", "/dev/loop10"), self.system.commands)
        self.assertFalse((Path(owner["directory"]) / "data").exists())

    def test_failed_truncate_still_has_cleanup_ownership(self):
        with patch.object(POOL.os, "ftruncate", side_effect=OSError("no space")):
            with self.assertRaisesRegex(OSError, "no space"):
                self.create()
        owner = json.loads((self.evidence / "ownership.json").read_text())
        self.assertTrue(owner["removed"])
        self.assertFalse((Path(owner["directory"]) / "data").exists())

    def test_ten_gib_actual_pool_rejected_and_owned_resources_removed(self):
        self.system.capacity = 10 * POOL.GIB
        with self.assertRaisesRegex(ValueError, "capacity"):
            self.create()
        self.assertFalse(self.system.exists)
        self.assertFalse(self.system.attached)
        self.assertTrue((self.evidence / "table.txt").exists())
        self.assertTrue((self.evidence / "status.txt").exists())

    def test_out_of_space_or_insufficient_free_blocks_rejected(self):
        table = self.system.table()
        plan = {"data_bytes": POOL.DATA_BYTES, "base_image_bytes": 10 * POOL.GIB}
        for status in ['0 83886080 thin-pool 0 1/262144 655360/655360 - out_of_data_space discard_passdown queue_if_no_space -',
                       '0 83886080 thin-pool 0 1/262144 500000/655360 - rw discard_passdown queue_if_no_space -']:
            with self.subTest(status=status), self.assertRaisesRegex(ValueError, "writable|capacity"):
                POOL.geometry(table, status, plan)

    def test_replaced_backing_refuses_cleanup(self):
        owner = self.create()
        data = Path(owner["directory"]) / "data"
        data.rename(data.with_name("original-data"))
        data.write_bytes(b"foreign")
        with self.assertRaisesRegex(ValueError, "identity changed"):
            POOL.remove(self.evidence / "ownership.json", self.system)
        self.assertTrue(self.system.exists)
        self.assertEqual(data.read_bytes(), b"foreign")

    def test_foreign_mapping_reference_prevents_loop_detach(self):
        self.create()
        self.system.foreign = 'foreign: 0 100 linear 7:10 0\n'
        with self.assertRaisesRegex(ValueError, "another mapping"):
            POOL.remove(self.evidence / "ownership.json", self.system)
        self.assertTrue(self.system.exists)

    def test_lookalike_snapshot_mapping_is_not_deleted(self):
        self.create()
        self.system.foreign = self.pool + '-snap-99: 0 100 thin 252:99 1\n'
        with self.assertRaisesRegex(ValueError, "another device"):
            POOL.remove(self.evidence / "ownership.json", self.system)
        self.assertTrue(self.system.exists)


if __name__ == "__main__":
    unittest.main()
