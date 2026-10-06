import json
import pathlib
import unittest
import copy

from validate_point import audit, formal_plan, messages, stream_pairs, vm_lifecycles


class StreamAssociationTest(unittest.TestCase):
    def row(self, kind, revision, size):
        message = f"ZSTD_WS_{kind} revision={revision} layout=streams8-v1"
        if kind == "DECODE":
            message += f" compressed_bytes={size}"
        else:
            message += " stats=" + json.dumps({"streams": [{"read_bytes": size}]})
        return "time=x msg=" + json.dumps(message)

    def test_interleaved_calls_join_by_revision(self):
        log = "\n".join([self.row("STREAM_STATS", "rev-a", 100),
                         self.row("STREAM_STATS", "rev-b", 200),
                         self.row("DECODE", "rev-b", 200),
                         self.row("DECODE", "rev-a", 100)])
        pairs = stream_pairs(messages(log), 2, "rev-")
        self.assertEqual([p[0] for p in pairs], ["rev-b", "rev-a"])
        for _, fields, sample in pairs:
            self.assertEqual(int(fields["compressed_bytes"]), sample["streams"][0]["read_bytes"])

    def test_duplicate_or_wrong_revision_is_rejected(self):
        for log in ("\n".join([self.row("DECODE", "rev-a", 100)] * 2),
                    self.row("DECODE", "other", 100),
                    "\n".join([self.row("DECODE", "rev-a", 100),
                               self.row("STREAM_STATS", "rev-b", 100)])):
            with self.assertRaises(ValueError):
                stream_pairs(messages(log), 1, "rev-")

    def test_native_has_no_ws_markers(self):
        self.assertEqual(stream_pairs([], 0, "rev-"), [])

    def test_saved_aes_evidence(self):
        root = pathlib.Path(__file__).parent / "verification/remote-20260910-r1/aes-gate2"
        if not root.is_dir():
            self.skipTest("optional archived functional evidence not packaged")
        result = audit((root / "relay.log").read_text(),
                       json.loads((root / "remote-fetch-stats.json").read_text()), 1, True,
                       "cold-aes-go-45000-45450-direct512-currentbase-ws5-20260902-currentbase17-r1-0")
        self.assertEqual(result["decoded_payload_bytes"], 1076057)
        self.assertEqual(len(result["decoded_revisions"]), 1)

    def test_frozen_manifest_not_self_reported_byte_total(self):
        root = pathlib.Path(__file__).parent / "verification/remote-20260910-r1/aes-gate2"
        inventory = pathlib.Path(__file__).parent / "provenance/20260910-r1/actual-layout.json"
        if not root.is_dir() or not inventory.exists():
            self.skipTest("optional actual corpus evidence not packaged")
        row = next(r for r in json.loads(inventory.read_text())["rows"]
                   if r["system"] == "splitsnap-zstd3" and r["profile"] == "aes-go-45000-45450")
        log, stats = (root / "relay.log").read_text(), json.loads((root / "remote-fetch-stats.json").read_text())
        audit(log, stats, 1, True, row["snapshot"], manifest=row["manifest"])
        wrong = copy.deepcopy(row["manifest"])
        # Same total, different per-stream boundaries must still fail.
        wrong["streams"][0]["compressed_size"] += 1
        wrong["streams"][1]["compressed_size"] -= 1
        with self.assertRaisesRegex(ValueError, "frozen manifest"):
            audit(log, stats, 1, True, row["snapshot"], manifest=wrong)


class LifecycleTest(unittest.TestCase):
    def fixture(self):
        values = json.dumps(dict(LoadVMM=10, GetWorkingSetPages=1, GetUffdMemoryContent=3, GetWorkingSetContent=5))
        msgs = [f"RESTORE_COMPONENTS revision=source vm_id=vm1 metrics_us={values}",
                "created VM with ID vm1 and IP 1.2.3.4 for revision source-123-00001",
                'Invocation to vm1 completed in 50ms with HTTP status 200 and gRPC status "0"',
                "Handled 8 page faults in 9ms",
                "VM_TERMINATION_CONFIRMED vm_id=vm1 method=graceful processes_absent=true uffd_released=true",
                "Stopped VM successfully"]
        return "\n".join("time=x level=debug msg=" + json.dumps(m) + " vmID=vm1 uffd=/tmp/vm1.uffd.sock" for m in msgs)

    def test_complete_identity_and_release(self):
        self.assertEqual(vm_lifecycles(self.fixture(), ["source"], True)["vms"], 1)

    def test_forced_stop_requires_termination_and_release_proof(self):
        log = self.fixture().replace("method=graceful", "method=forced")
        self.assertEqual(vm_lifecycles(log, ["source"], True)["forced_terminations"], 1)
        for bad in (log.replace("processes_absent=true", "processes_absent=false"),
                    log.replace("uffd_released=true", "uffd_released=false")):
            with self.assertRaises(ValueError):
                vm_lifecycles(bad, ["source"], True)

    def test_wrong_vm_missing_stop_and_ignored_error_are_rejected(self):
        for log in (self.fixture().replace("vm_id=vm1", "vm_id=another"),
                    self.fixture().replace("Stopped VM successfully", "stopping"),
                    self.fixture() + '\nlevel=error msg="failed to stop firecracker-containerd VM" vmID=vm1'):
            with self.assertRaises(ValueError):
                vm_lifecycles(log, ["source"], True)


class FormalPlanTest(unittest.TestCase):
    def fixture(self):
        row = dict(system="ws-zstd3", profile="aes", snapshot="source", corpus_id="corpus", endpoint="backend:9562",
                   security="full", tier=512, chunk_size=4096, ws_layout="streams8-v1")
        entry = dict(key="source/snap_file", size=17, etag="etag")
        inv = dict(run_id="run", layout="streams8-v1", rows=[row] + [dict(system="other", profile=str(i)) for i in range(101)],
                   alias_source_entries={"corpus": {entry["key"]: entry}})
        aliases = dict(run_id="run", layout="streams8-v1", tag="streams8-run", materialized=True, aliases=[
            dict(endpoint=row["endpoint"], source_revision="source", alias_revision=f"source-streams8-run-{i}",
                 slot=i, corpus_id="corpus", source_entries=[entry]) for i in range(60)])
        rows = [dict(index=str(i+1), slot=str(i), revision=f"source-streams8-run-{i}-123-{i+1:05d}") for i in range(60)]
        cfg = dict(run_id="run", minio_endpoint=row["endpoint"], security="full", vm_mib=512,
                   chunk_size=4096, ws_coalescing=True)
        return rows, inv, aliases, cfg

    def test_exact_plan_accepts_interleaved_completion(self):
        rows, inv, aliases, cfg = self.fixture()
        point, revisions = formal_plan(rows[::-1], inv, aliases, cfg, "ws-zstd3", "aes")
        self.assertEqual(point["snapshot"], "source")
        self.assertEqual(len(revisions), 60)

    def test_wrong_slots_aliases_corpus_or_unmaterialized_report_fail(self):
        for mutation in ("slot", "alias", "endpoint", "source", "preflight"):
            with self.subTest(mutation=mutation):
                rows, inv, aliases, cfg = self.fixture()
                if mutation == "slot":
                    rows[0]["slot"] = "59"
                elif mutation == "alias":
                    rows[0]["revision"] = "wrong-streams8-run-0-123-00001"
                elif mutation == "endpoint":
                    cfg["minio_endpoint"] = "old:9462"
                elif mutation == "source":
                    aliases["aliases"][0]["source_entries"] = [dict(key="source/snap_file", size=18, etag="etag")]
                else:
                    aliases["materialized"] = False
                with self.assertRaises(ValueError):
                    formal_plan(rows, inv, aliases, cfg, "ws-zstd3", "aes")


if __name__ == "__main__":
    unittest.main()
