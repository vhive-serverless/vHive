import unittest
import pathlib
import tempfile
import json
from unittest.mock import patch, call

from run_matrix import Matrix, ordered_points, wait_job, collect_window, collect_resources, require_versioned_negative_gate


class MatrixTest(unittest.TestCase):
    def test_versioned_gates_require_late_cancellation_and_pool_exit(self):
        for version in ("gate11", "gate12"):
            with self.subTest(version=version):
                self.check_late_cancellation_and_pool_exit(version)

    def check_late_cancellation_and_pool_exit(self, version):
        with tempfile.TemporaryDirectory() as tmp:
            gates = pathlib.Path(tmp)
            root, late_root = gates/("aes-negative-"+version), gates/("aes-cancel-after-load-"+version)
            root.mkdir(); late_root.mkdir()
            binary = "/owned/bin/relay-streams8-"+version
            state = dict(active_requests=0, active_uffd_handlers=0, active_vm_ids=[], cleanup_tasks=0,
                         pending_shim_refills=0, pending_ws_cache_writes=0)
            (root/"negative-validation.json").write_text(json.dumps(dict(accepted=True,released_vm="vm1")))
            (root/"launch.json").write_text(json.dumps(dict(config=dict(relay_binary=binary))))
            (root/"final.json").write_text(json.dumps(dict(binary=binary,runtime=state)))
            shutdown = dict(binary=binary,processes_absent=True,remaining_processes=[],vm_ids=["vm1","vm2","unused-vm"])
            (root/"shutdown.json").write_text(json.dumps(shutdown))
            late = dict(binary=binary,accepted=True,cancelled_after_load=True,uffd_released=True,runtime_idle=True,vm_id="vm2")
            (late_root/"validation.json").write_text(json.dumps(late))
            require_versioned_negative_gate(gates,version)
            shutdown["remaining_processes"] = [dict(pid=123,vm_id="unused-vm")]
            (root/"shutdown.json").write_text(json.dumps(shutdown))
            with self.assertRaisesRegex(ValueError,"pool shutdown"):
                require_versioned_negative_gate(gates,version)

    def test_negative_gate_requires_current_binary_and_idle(self):
        with tempfile.TemporaryDirectory() as tmp:
            gates = pathlib.Path(tmp)
            root = gates / "aes-negative-gate10"
            root.mkdir()
            binary = "/owned/bin/relay-streams8-gate10"
            state = dict(active_requests=0, active_uffd_handlers=0, active_vm_ids=[], cleanup_tasks=0,
                         pending_shim_refills=0, pending_ws_cache_writes=0)
            (root/"negative-validation.json").write_text(json.dumps(dict(accepted=True)))
            (root/"launch.json").write_text(json.dumps(dict(config=dict(relay_binary=binary))))
            (root/"final.json").write_text(json.dumps(dict(binary=binary, runtime=state)))
            require_versioned_negative_gate(gates, "gate10")
            (root/"final.json").write_text(json.dumps(dict(binary=binary.replace("gate10", "gate9"), runtime=state)))
            with self.assertRaisesRegex(ValueError, "binary/release differs"):
                require_versioned_negative_gate(gates, "gate10")
            state["active_uffd_handlers"] = 1
            (root/"final.json").write_text(json.dumps(dict(binary=binary, runtime=state)))
            with self.assertRaisesRegex(ValueError, "binary/release differs"):
                require_versioned_negative_gate(gates, "gate10")

    def test_resource_record_is_bound_to_ended_owned_unit(self):
        with tempfile.TemporaryDirectory() as tmp:
            point = pathlib.Path(tmp)
            record = {"MESSAGE":"owned.service: Consumed 5.1s CPU time, 12M memory peak, 0B memory swap peak.","__REALTIME_TIMESTAMP":"123"}
            with patch("run_matrix.remote", return_value=json.dumps(record)) as fetch:
                collect_resources(point, "owned.service")
                collect_resources(point, "owned.service")
                fetch.assert_called_once()
            with self.assertRaisesRegex(ValueError, "unit differs"):
                collect_resources(point, "another.service")
            with patch("run_matrix.remote", return_value=""), self.assertRaisesRegex(ValueError, "missing/ambiguous"):
                collect_resources(point/"missing", "owned.service")

    def test_window_bundle_uses_one_stream_and_rejects_partial_transfer(self):
        for extraction_rc, sender_rc in ((0,0), (1,0), (0,1)):
            with patch("run_matrix.subprocess.Popen") as popen, patch("run_matrix.subprocess.run") as run:
                sender = popen.return_value.__enter__.return_value
                sender.wait.return_value = sender_rc
                run.return_value.returncode = extraction_rc
                if extraction_rc or sender_rc:
                    with self.assertRaisesRegex(RuntimeError, "do not replay calls"):
                        collect_window("/owned/window", pathlib.Path("/local/point"))
                else:
                    collect_window("/owned/window", pathlib.Path("/local/point"))
                self.assertEqual(popen.call_args.args[0][-1], "tar -czf - -C /owned/window .")
                self.assertEqual(run.call_args.args[0], ["tar", "-xzf", "-", "-C", "/local/point"])
                sender.stdout.close.assert_called_once()

    def test_full_scope_and_matched_local(self):
        profiles = ["aes-go-45000-45450"] + [f"profile{i}" for i in range(16)]
        plan = {"workloads": {"workloads": [{"profile": p} for p in profiles]}}
        points = ordered_points(plan)
        self.assertEqual(len(points), 103)
        self.assertEqual(len(set(points)), 103)
        self.assertEqual(points[-1], ("splitsnap-zstd3", "aes-go-45000-45450", "local"))
        self.assertEqual(sum(mode == "remote" for _, _, mode in points), 102)

    def test_unknown_handle_is_not_replayed(self):
        for status in ("UNKNOWN", "ABSENT", "RUNNING 123 1"):
            with patch("run_matrix.job_status", return_value=status), self.assertRaises(RuntimeError):
                wait_job("loader", "/job", "session")

    def test_live_job_is_observed_until_terminal(self):
        with patch("run_matrix.job_status", side_effect=["RUNNING 123 0", "DONE 7"]), patch("run_matrix.time.sleep"):
            self.assertEqual(wait_job("loader", "/job", "session"), 7)

    def test_idle_failure_preserves_logs_without_replaying(self):
        runner = Matrix.__new__(Matrix)
        with tempfile.TemporaryDirectory() as directory, patch("run_matrix.copy") as transfer, \
                patch.object(runner, "probe", side_effect=[RuntimeError("VM still active"),
                    {"diagnostic_only": True, "settled": False, "runtime": {"active_vm_ids": ["vm1"]}}]) as probe:
            root = pathlib.Path(directory)
            with self.assertRaisesRegex(RuntimeError, "VM still active"):
                runner.collect_final(root, {"log_dir": "/owned/logs"}, "/owned/config")
            self.assertEqual(probe.call_args_list, [call("/owned/config", "final"), call("/owned/config", "diagnostic")])
            self.assertEqual([c.args[2].name for c in transfer.call_args_list],
                             ["relay-before-final.log", "launch.json", "relay.log"])
            self.assertEqual(len(list(root.glob("collection-error-*.json"))), 1)
            diagnostic = json.loads(next(root.glob("unsettled-diagnostic-*.json")).read_text())
            self.assertTrue(diagnostic["diagnostic_only"])
            self.assertFalse(diagnostic["settled"])
