import json
import pathlib
import unittest

from launcher import relay_command
from point_config import point_config


class PointConfigTest(unittest.TestCase):
    def setUp(self):
        self.plan = json.loads((pathlib.Path(__file__).parent / "config/20260910-r1/plan.json").read_text())

    def test_entire_102_matrix_resolves(self):
        seen = set()
        for row in self.plan["configuration_rows"]:
            c = point_config(self.plan, row["system"], row["profile"], "r1")
            seen.add(c["point_id"])
            self.assertEqual(c["vm_mib"], row["tier"])
            self.assertEqual(c["security"], row["security"])
            args = relay_command(c)
            self.assertEqual("-wsCompression" in args, row["ws_streams"] == 8)
            self.assertFalse(any("FrameSize" in a for a in args))
        self.assertEqual(len(seen), 102)

    def test_oracle_partial_and_aes_matched_local(self):
        c = point_config(self.plan, "full-dedup-zstd3", "aes-go-45000-45450", "r1")
        self.assertEqual(c["security"], "partial")
        remote = point_config(self.plan, "splitsnap-zstd3", "aes-go-45000-45450", "r1")
        local = point_config(self.plan, "splitsnap-zstd3", "aes-go-45000-45450", "r1", "local")
        for key in ("minio_endpoint", "security", "ws_coalescing", "chunk_size", "vm_mib"):
            self.assertEqual(remote[key], local[key])
        with self.assertRaises(ValueError):
            point_config(self.plan, "ws-zstd3", "aes-go-45000-45450", "r1", "local")


if __name__ == "__main__":
    unittest.main()
