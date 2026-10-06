"""Offline recheck of saved per-point platform observations before publication.

Mirror the frozen gate11 platform_probe contract without querying or changing
the running nodes. Before/after observations do not prove continuous monitoring.
"""
import json
import re


ROLES = ("worker", "backend", "loader")
POLICY = {"scaling_governor": "performance", "scaling_min_freq": "2100000",
          "scaling_max_freq": "2100000"}


def validate_snapshot(role, data):
    if data["role"] != role or "5512U" not in data["cpu_model"]:
        raise ValueError("platform role/CPU differs")
    online = data["online_cpus"]
    if (data["smt_active"] != "0" or data["no_turbo"] != "1" or
            len(online) != 28 or len(set(online)) != 28 or len(data["policies"]) != 28 or
            any(policy != POLICY for policy in data["policies"].values())):
        raise ValueError("platform CPU frequency/SMT/Turbo/online count differs")
    if role == "worker":
        if not re.search(r"^qdisc tbf .*rate 10Gbit .*lat 400ms", data["qdisc"], re.M):
            raise ValueError("platform worker IFB rate/latency differs")
        filters = data["ingress_filter"]
        if any(token not in filters for token in ("ifb0", "protocol all", "match 00000000/00000000 at 0")):
            raise ValueError("platform worker match-all redirect differs")
    elif role == "backend" and re.search(r"^qdisc tbf ", data["qdisc"], re.M):
        raise ValueError("platform backend duplicates shaping")


def validate_point_platform(point):
    before, after = [json.loads((point / f"platform-{phase}.json").read_text())
                     for phase in ("before", "after")]
    for snapshots in (before, after):
        if set(snapshots) != set(ROLES):
            raise ValueError("platform three-node observations incomplete")
        for role in ROLES:
            validate_snapshot(role, snapshots[role])
    for role in ROLES:
        if after[role]["captured_at"] < before[role]["captured_at"]:
            raise ValueError("platform before/after order differs")
    return 2 * len(ROLES)
