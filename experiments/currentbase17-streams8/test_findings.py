import unittest

from findings import build_findings


class FindingsTest(unittest.TestCase):
    def test_raw_mean_reduction_and_regression_are_preserved(self):
        profiles = ["small", "large"]
        systems = ("chunks-128k-zstd3", "pages-4k-zstd3", "ws-zstd3", "no-image-zstd3",
                   "splitsnap-zstd3", "full-dedup-zstd3")
        summaries = {(s, p, "remote"): {"relay_e2e": 2 if p == "small" else 100}
                     for s in systems for p in profiles}
        summaries["splitsnap-zstd3", "small", "remote"]["relay_e2e"] = 4
        summaries["splitsnap-zstd3", "large", "remote"]["relay_e2e"] = 50
        reports = {"footprint": {"systems": [
            dict(system="WS", total_storage_bytes=100, active_cache_bytes=100),
            dict(system="SplitSnap", total_storage_bytes=60, active_cache_bytes=70)]},
            "payload": {"rows": [dict(system=s, profile=p, compressed_payload_bytes=1 if s=="splitsnap-zstd3" else 2)
                                 for s in systems for p in profiles]}}
        text = build_findings(profiles, summaries, reports)
        self.assertIn("| Sabre | 51.000000 | 47.059 |", text)
        self.assertIn("| Sabre | 1 / 0 / 1 | -100.000 / 50.000 |", text)
        self.assertIn("| small | 2.000000 | 2.000000 | 4.000000 | -100.000 | -100.000 |", text)
        self.assertIn("| Remote storage | 40.000 |", text)
        self.assertIn("| Local cache | 30.000 |", text)
        self.assertIn("| Mean fetch payload | 50.000 |", text)
        self.assertIn("不是旧小帧与新八流的matched布局A/B", text)


if __name__ == "__main__":
    unittest.main()
