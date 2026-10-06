import csv
import json
import pathlib
import unittest

from metrics import duration_ms, medians, parse_metrics, quality_summary


class MetricIdentityTest(unittest.TestCase):
    def synthetic(self, coalesced, mode):
        calls, lines = [], []
        def emit(msg, vm=None):
            lines.append('msg=' + json.dumps(msg) + (f' uffd=/tmp/{vm}.uffd.sock' if vm else ''))
        for slot in range(60):
            vm, alias = f"vm-{slot}", f"alias-{slot}"
            calls.append(dict(slot=str(slot), revision=alias+"-nonce-5"))
            emit(f"created VM with ID {vm} and IP 10.0.0.1 for revision {alias}-nonce-5")
            values = dict(LoadVMM=1000, GetWorkingSetPages=2000, GetUffdMemoryContent=3000)
            if coalesced: values["GetWorkingSetContent"] = 4000
            emit(f"RESTORE_COMPONENTS revision={alias} vm_id={vm} metrics_us="+json.dumps(values))
            if mode == "remote": emit(f"Downloaded snapshot for rev {alias} in 5000")
            if coalesced: emit(f"ZSTD_WS_DECODE revision={alias} layout=streams8-v1 elapsed_us=3500")
            emit("Pre-inserting working set of 100 pages in 1ms" + (", private page count: 40" if coalesced else ""), vm)
            emit("Handled 12 page faults in 2ms", vm)
            emit(f"Invocation to {vm} completed in {slot+20}ms with HTTP status 200")
        return "\n".join(reversed(lines)), list(reversed(calls))

    def test_native_remote_and_strict_local_metrics(self):
        for coalesced, mode in ((False, "remote"), (True, "local")):
            with self.subTest(coalesced=coalesced, mode=mode):
                log, calls = self.synthetic(coalesced, mode)
                summary = medians(parse_metrics(log, calls, coalesced, mode))
                self.assertEqual(summary["relay_e2e"], 64.5)
                self.assertEqual(summary["download"], 0 if mode=="local" else 5)
                self.assertEqual(summary["get_ws_content"], 4 if coalesced else 0)
                self.assertEqual(summary["ws_decode_nested"], 3.5 if coalesced else 0)
                self.assertEqual(summary["page_fault_count"], 12)

    def test_quality_preserves_tail_and_validates_raw_page_counts(self):
        log, calls = self.synthetic(True, "remote")
        samples = parse_metrics(log, calls, True, "remote")
        inventory = dict(working_set_pages=100,index_pages=40,raw_bytes=40*4096,recipe_bytes=2**21,tier=512)
        samples[-1]["page_fault_count"] = 201
        result = quality_summary(samples, inventory)
        self.assertEqual(result["median_extra_events"], 11)
        self.assertEqual(result["max_extra_events"], 200)
        self.assertEqual(result["calls_over_100_extra_events"], 1)
        self.assertEqual(result["whole_ws_pct"], 11)
        samples[0]["private_ws_pages"] += 1
        with self.assertRaisesRegex(ValueError, "frozen WS"):
            quality_summary(samples, inventory)

    def evidence(self):
        root = pathlib.Path(__file__).parent / "results/20260910-r1-gate7/points/splitsnap-zstd3--aes-go-45000-45450--remote"
        if not root.is_dir():
            self.skipTest("optional archived diagnostic fixture")
        with (root/"invocations.tsv").open() as f:
            rows = list(csv.DictReader(f, delimiter="\t"))
        return (root/"relay.log").read_text(), rows

    def test_interleaving_and_completion_order_do_not_change_association(self):
        log, rows = self.evidence()
        parsed = parse_metrics(log, rows, True, "remote")
        reversed_log = "\n".join(reversed(log.splitlines()))
        self.assertEqual(parsed, parse_metrics(reversed_log, list(reversed(rows)), True, "remote"))
        self.assertEqual([r["slot"] for r in parsed], list(range(60)))
        self.assertGreater(medians(parsed)["relay_e2e"], 0)

    def test_missing_keyed_component_cannot_use_anonymous_fallback(self):
        log, rows = self.evidence()
        with self.assertRaises(ValueError):
            parse_metrics(log.replace("RESTORE_COMPONENTS", "IGNORED_COMPONENTS", 1), rows, True, "remote")

    def test_go_duration(self):
        self.assertAlmostEqual(duration_ms("1m2.5s"), 62500)
        self.assertAlmostEqual(duration_ms("27µs"), .027)
        with self.assertRaises(ValueError):
            duration_ms("27 bogus")
