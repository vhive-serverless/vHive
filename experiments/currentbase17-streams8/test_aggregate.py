import json
import pathlib
import tempfile
import unittest
from unittest.mock import patch

import aggregate


class AggregateTest(unittest.TestCase):
    def test_formal_data_rechecks_saved_platform_before_metrics(self):
        s, p, mode = "ws-zstd3", "test-profile", "remote"
        plan = {"test": "frozen plan"}
        with tempfile.TemporaryDirectory() as tmp:
            root = pathlib.Path(tmp)
            (root/"frozen.json").write_text(json.dumps(dict(version="gate11", plan=plan, inventory={}, aliases={})))
            point = root/"points"/(s+"--"+p+"--"+mode)
            point.mkdir(parents=True)
            (point/"POINT_COMPLETE.json").write_text(json.dumps(dict(system=s, profile=p, mode=mode, version="gate11")))
            (point/"config.json").write_text("{}")
            with patch.object(aggregate, "ordered_points", return_value=[(s,p,mode)]), \
                    patch.object(aggregate, "validate_point_platform", side_effect=ValueError("platform changed")) as check, \
                    patch.object(aggregate, "parse_metrics") as parse:
                with self.assertRaisesRegex(ValueError, "platform changed"):
                    aggregate.formal_data(root, plan)
            check.assert_called_once_with(point)
            parse.assert_not_called()

    def test_mean_is_ratio_of_raw_means_not_mean_of_ratios(self):
        profiles = ["small", "large"]
        values = {(s, p): (1 if p=="small" else 90) for s in aggregate.SYSTEMS for p in profiles}
        denominator, selected = aggregate.SYSTEMS[:2]
        values[denominator, "small"], values[denominator, "large"] = 2, 100
        rows = aggregate.normalized_rows(values, [selected], profiles, denominator)
        self.assertAlmostEqual(rows[-1]["normalized"], 91/102)
        self.assertNotAlmostEqual(rows[-1]["normalized"], (.5+.9)/2)

    def test_invalid_formal_evidence_does_not_create_partial_pack(self):
        with tempfile.TemporaryDirectory() as tmp:
            out = pathlib.Path(tmp)/"result"
            with patch("sys.argv", ["aggregate.py", "--results", tmp, "--output", str(out)]), \
                    patch.object(aggregate, "formal_data", side_effect=ValueError("incomplete formal points")):
                with self.assertRaisesRegex(ValueError, "incomplete formal points"):
                    aggregate.main()
            self.assertFalse(out.exists())

    def test_static_preview_does_not_export_formal_findings(self):
        with tempfile.TemporaryDirectory() as tmp:
            out = pathlib.Path(tmp)/"static"
            with patch("sys.argv", ["aggregate.py", "--static-only", "--output", str(out)]), \
                    patch.object(aggregate, "load_accounting", return_value={}), \
                    patch.object(aggregate, "static_figures"), \
                    patch.object(aggregate, "build_findings") as findings:
                aggregate.main()
            findings.assert_not_called()
            self.assertFalse((out/"FINDINGS.md").exists())

    def test_formal_export_links_findings_after_acceptance(self):
        with tempfile.TemporaryDirectory() as tmp:
            out = pathlib.Path(tmp)/"formal"
            summaries, reports = {}, {}
            with patch("sys.argv", ["aggregate.py", "--results", tmp, "--output", str(out)]), \
                    patch.object(aggregate, "formal_data", return_value=(summaries, [], [], [], [])), \
                    patch.object(aggregate, "frozen_accounting", return_value=reports), \
                    patch.object(aggregate, "static_figures"), \
                    patch.object(aggregate, "formal_figures"), \
                    patch.object(aggregate, "build_findings", return_value="accepted comparison\n") as findings:
                aggregate.main()
            self.assertIs(findings.call_args.args[1], summaries)
            self.assertIs(findings.call_args.args[2], reports)
            self.assertEqual((out/"FINDINGS.md").read_text(), "accepted comparison\n")
            self.assertIn("[FINDINGS.md](FINDINGS.md)", (out/"README.md").read_text())


if __name__ == "__main__":
    unittest.main()
