import copy
import json
import pathlib
import tempfile
import unittest

from platform_evidence import POLICY, ROLES, validate_point_platform, validate_snapshot


def snapshot(role):
    data = dict(role=role, cpu_model="Intel Xeon Gold 5512U", captured_at=100,
                smt_active="0", no_turbo="1", online_cpus=list(range(28)),
                policies={f"policy{i}": dict(POLICY) for i in range(28)})
    if role == "worker":
        data.update(qdisc="qdisc tbf 1: root rate 10Gbit burst 4Mb lat 400ms",
                    ingress_filter="protocol all match 00000000/00000000 at 0 redirect dev ifb0")
    elif role == "backend":
        data["qdisc"] = "qdisc mq 0: root"
    return data


class PlatformEvidenceTest(unittest.TestCase):
    def test_saved_pair_requires_all_six_observations(self):
        with tempfile.TemporaryDirectory() as tmp:
            point = pathlib.Path(tmp)
            before = {role: snapshot(role) for role in ROLES}
            after = copy.deepcopy(before)
            for data in after.values():
                data["captured_at"] = 200
            (point/"platform-before.json").write_text(json.dumps(before))
            (point/"platform-after.json").write_text(json.dumps(after))
            self.assertEqual(validate_point_platform(point), 6)
            del after["loader"]
            (point/"platform-after.json").write_text(json.dumps(after))
            with self.assertRaisesRegex(ValueError, "incomplete"):
                validate_point_platform(point)
            (point/"platform-after.json").unlink()
            with self.assertRaises(FileNotFoundError):
                validate_point_platform(point)

    def test_cpu_and_network_drift_rejected(self):
        for field, value in (("role", "loader"), ("cpu_model", "Other CPU"),
                             ("smt_active", "1"), ("no_turbo", "0"),
                             ("online_cpus", [0]*28), ("policies", {}),
                             ("qdisc", "qdisc tbf 1: root rate 1Gbit lat 400ms"),
                             ("ingress_filter", "redirect dev ifb0")):
            with self.subTest(field=field):
                data = snapshot("worker")
                data[field] = value
                with self.assertRaises(ValueError):
                    validate_snapshot("worker", data)
        data = snapshot("worker")
        data["policies"]["policy0"]["scaling_max_freq"] = "2700000"
        with self.assertRaisesRegex(ValueError, "frequency"):
            validate_snapshot("worker", data)
        data = snapshot("backend")
        data["qdisc"] = "qdisc tbf 1: root rate 10Gbit lat 400ms"
        with self.assertRaisesRegex(ValueError, "duplicates"):
            validate_snapshot("backend", data)

    def test_reversed_observation_order_rejected(self):
        with tempfile.TemporaryDirectory() as tmp:
            point = pathlib.Path(tmp)
            before = {role: snapshot(role) for role in ROLES}
            after = copy.deepcopy(before)
            after["worker"]["captured_at"] = 99
            for phase, data in (("before", before), ("after", after)):
                (point/f"platform-{phase}.json").write_text(json.dumps(data))
            with self.assertRaisesRegex(ValueError, "order"):
                validate_point_platform(point)


if __name__ == "__main__":
    unittest.main()
