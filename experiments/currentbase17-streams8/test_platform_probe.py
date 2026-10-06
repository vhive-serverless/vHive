import pathlib
import tempfile
import unittest

from platform_probe import online_policies


class PlatformProbeTest(unittest.TestCase):
    def test_offline_sibling_policy_not_read(self):
        with tempfile.TemporaryDirectory() as directory:
            root = pathlib.Path(directory)
            (root / "online").write_text("0-1,4\n")
            for cpu in (0, 1, 4):
                policy = root / "cpufreq" / f"policy{cpu}"
                policy.mkdir(parents=True)
                for name, value in (("scaling_governor", "performance"),
                                    ("scaling_min_freq", "2100000"), ("scaling_max_freq", "2100000")):
                    (policy / name).write_text(value)
                (root / f"cpu{cpu}").mkdir()
                (root / f"cpu{cpu}" / "cpufreq").symlink_to(policy)
            # Deliberately incomplete: reading offline policy2 must fail the test.
            (root / "cpufreq/policy2").mkdir()
            cpus, policies = online_policies(root)
            self.assertEqual(cpus, [0, 1, 4])
            self.assertEqual(set(policies), {"policy0", "policy1", "policy4"})


if __name__ == "__main__":
    unittest.main()
