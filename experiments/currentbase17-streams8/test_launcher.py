import socket
import unittest
import pathlib
import tempfile
from unittest.mock import patch

from launcher import check_endpoint_available, checked_config, relay_command, owned_vm_ids, remaining_vm_processes


class LauncherTest(unittest.TestCase):
    def test_shutdown_procfs_exit_race_not_permission_failure(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = pathlib.Path(tmp)
            (root/"128022").mkdir()
            for error in (FileNotFoundError(), ProcessLookupError()):
                with self.subTest(error=type(error).__name__), \
                        patch.object(pathlib.Path, "read_bytes", side_effect=error):
                    self.assertEqual(remaining_vm_processes(["shim-123-1"], root), [])
            with patch.object(pathlib.Path, "read_bytes", side_effect=PermissionError()):
                with self.assertRaises(PermissionError):
                    remaining_vm_processes(["shim-123-1"], root)

    def test_shutdown_checks_unused_pool_and_exact_process_identity(self):
        ids = owned_vm_ids("created vmID=shim-123-1; Creating new shim vmID=shim-123-2", 123)
        self.assertEqual(ids, ["shim-123-1", "shim-123-2"])
        for log in ("no IDs", "vmID=shim-124-1"):
            with self.assertRaises(ValueError):
                owned_vm_ids(log, 123)
        with tempfile.TemporaryDirectory() as tmp:
            root = pathlib.Path(tmp)
            for pid, cmd in (("8", "/usr/bin/firecracker\0--id\0shim-123-2\0"),
                             ("9", "/usr/bin/firecracker\0--id\0shim-123-20\0"),
                             ("10", "/usr/bin/bash\0--id\0shim-123-1\0")):
                (root/pid).mkdir()
                (root/pid/"cmdline").write_bytes(cmd.encode())
            self.assertEqual(remaining_vm_processes(ids, root), [dict(pid=8, vm_id="shim-123-2")])

    def config(self):
        return dict(run_id="r1", point_id="aes-split", run_root="/opt/streams8/r1",
                    cache_dir="/opt/streams8/r1/aes/snapshots", scratch_dir="/opt/streams8/r1/aes/scratch",
                    log_dir="/opt/streams8/r1/aes/logs", network_name_prefix="sstr",
                    relay_binary="/opt/streams8/bin/relay", relay_cwd="/opt/streams8/relay",
                    images_dir="/users/Liquidz/images", relay_endpoint="10.0.1.1:8090", ws_cache_endpoint="10.0.1.1:8091",
                    minio_endpoint="10.0.1.2:9565", security="partial", mode="remote",
                    vm_mib=512, chunk_size=4096, clean=True, ws_coalescing=True, base_snap=True,
                    veth_prefix="172.30", clone_prefix="172.31", host_iface="eth0", dns="10.0.1.2")

    def test_coalesced_and_native_commands(self):
        c = self.config()
        for coalescing in (True, False):
            c["ws_coalescing"] = coalescing
            args = relay_command(c)
            self.assertEqual("-wsCompression" in args, coalescing)
            self.assertEqual("-wsCoalescing" in args, coalescing)
            self.assertFalse(any("FrameSize" in arg for arg in args))
            self.assertIn("-j=16", args)
            self.assertIn("-zstdFetchers=8", args)
            self.assertIn("-endpoint=10.0.1.1:8090", args)
            self.assertIn("-snapshotsDir=/opt/streams8/r1/aes/snapshots", args)
            self.assertIn("-snapshotsScratchDir=/opt/streams8/r1/aes/scratch", args)

    def test_rejects_shared_paths_and_legacy_endpoint(self):
        for key, value in (("cache_dir", "/users/Liquidz/snapshots"),
                           ("scratch_dir", "/opt/streams8/r1/aes/snapshots"),
                           ("relay_endpoint", "10.0.1.1:8080"), ("network_name_prefix", ""),
                           ("security", "full-dedup")):
            c = self.config()
            c[key] = value
            with self.assertRaises(ValueError):
                checked_config(c)

    def test_all_local_is_same_compressed_representation(self):
        c = self.config()
        c.update(mode="local", clean=False)
        args = relay_command(c)
        self.assertIn("-wsCompression", args)
        self.assertIn("-snapshots=local", args)
        self.assertIn("-cacheSnaps=true", args)
        self.assertNotIn("-clean", args)

    def test_endpoint_rejects_a_live_listener(self):
        with socket.socket() as listener:
            listener.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
            listener.bind(("127.0.0.1", 0))
            listener.listen(1)
            endpoint = "127.0.0.1:" + str(listener.getsockname()[1])
            with self.assertRaises(OSError):
                check_endpoint_available(endpoint)

    def test_endpoint_allows_previous_listener_time_wait(self):
        with socket.socket() as listener, socket.socket() as client:
            listener.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
            listener.bind(("127.0.0.1", 0))
            address = listener.getsockname()
            listener.listen(1)
            client.connect(address)
            connection, _ = listener.accept()
            # The accepted/server side actively closes and enters TIME_WAIT.
            connection.close()
            self.assertEqual(client.recv(1), b"")
        with socket.socket() as plain_probe:
            with self.assertRaises(OSError):
                plain_probe.bind(address)
        check_endpoint_available("127.0.0.1:" + str(address[1]))


if __name__ == "__main__":
    unittest.main()
