import copy
import json
import pathlib
import tempfile
import unittest

from accounting_inputs import frozen_accounting, validate_accounting


class AccountingInputsTest(unittest.TestCase):
    def setUp(self):
        self.inventory = dict(run_id="run", layout="streams8-v1", rows=[dict(
            system="ws", profile="aes", snapshot="snap", endpoint="backend:1",
            ws_layout="streams8-v1", payload_key="snap/ws", compressed_bytes=12)])
        self.reports = dict(footprint=dict(layout="streams8-v1", profiles=["aes"]),
            payload=dict(layout="streams8-v1", run_id="run", object_sizes={"backend:1/snap/ws": 12},
                         rows=[dict(system="ws", profile="aes", snapshot="snap", endpoint="backend:1",
                                    layout="streams8-v1", payload_keys=["snap/ws"], unique_payload_objects=1,
                                    compressed_payload_bytes=12)]))

    def test_identity_and_actual_sizes(self):
        validate_accounting(self.reports, self.inventory)
        for field, value in (("snapshot", "other"), ("endpoint", "other:1"),
                             ("compressed_payload_bytes", 13)):
            reports = copy.deepcopy(self.reports)
            reports["payload"]["rows"][0][field] = value
            with self.subTest(field=field), self.assertRaises(ValueError):
                validate_accounting(reports, self.inventory)

    def test_native_objects_are_not_required_to_be_coalesced(self):
        self.inventory["rows"][0].update(ws_layout="not-applicable", payload_key="", compressed_bytes=0)
        self.reports["payload"]["rows"][0]["layout"] = "not-applicable"
        validate_accounting(self.reports, self.inventory)

    def test_formal_reads_embedded_reports_without_live_provenance(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = pathlib.Path(tmp)
            (root/"frozen.json").write_text(json.dumps(dict(inventory=self.inventory, accounting=self.reports)))
            # No provenance directory exists: the embedded exact reports suffice.
            self.assertEqual(frozen_accounting(root), self.reports)
            frozen = dict(inventory=self.inventory)
            (root/"frozen.json").write_text(json.dumps(frozen))
            with self.assertRaises(KeyError):
                frozen_accounting(root)
