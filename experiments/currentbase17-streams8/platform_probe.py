"""Read-only verification of the accepted c6620/2.1-GHz/worker-IFB platform."""
import argparse
import json
import pathlib
import re
import subprocess
import time


def online_policies(root):
    # Offline SMT siblings retain policy directories whose frequency files return
    # EBUSY. Resolve policies from the kernel's online CPU list, not glob(policy*).
    cpus = set()
    for part in (root / "online").read_text().strip().split(","):
        bounds = [int(n) for n in part.split("-")]
        cpus.update(range(bounds[0], bounds[-1] + 1))
    paths = {(root / f"cpu{cpu}/cpufreq").resolve(strict=True) for cpu in cpus}
    return sorted(cpus), {p.name: {k: (p / k).read_text().strip() for k in (
        "scaling_governor", "scaling_min_freq", "scaling_max_freq")} for p in sorted(paths)}


def capture(role):
    root = pathlib.Path("/sys/devices/system/cpu")
    smt = (root / "smt/active").read_text().strip()
    turbo = (root / "intel_pstate/no_turbo").read_text().strip()
    online, policies = online_policies(root)
    if smt != "0" or turbo != "1" or len(online) != 28 or len(policies) != 28 or any(v != {
        "scaling_governor": "performance", "scaling_min_freq": "2100000", "scaling_max_freq": "2100000"} for v in policies.values()):
        raise RuntimeError("CPU baseline differs; no settings were changed")
    result = dict(role=role, captured_at=time.time(), smt_active=smt, no_turbo=turbo, online_cpus=online, policies=policies,
                  cpu_model=subprocess.check_output(["lscpu"], text=True))
    if "5512U" not in result["cpu_model"]:
        raise RuntimeError("not the accepted c6620 CPU")
    if role == "worker":
        qdisc = subprocess.check_output(["tc", "qdisc", "show", "dev", "ifb0"], text=True)
        filters = subprocess.check_output(["tc", "filter", "show", "dev", "enp23s0f0np0", "ingress"], text=True)
        if not re.search(r"^qdisc tbf .*rate 10Gbit .*lat 400ms", qdisc, re.M):
            raise RuntimeError("worker IFB rate/latency differs")
        if "ifb0" not in filters or "protocol all" not in filters or "match 00000000/00000000 at 0" not in filters:
            raise RuntimeError("expected match-all ingress redirect, including the new MinIO ports")
        result.update(qdisc=qdisc, ingress_filter=filters)
    elif role == "backend":
        qdisc = subprocess.check_output(["tc", "qdisc", "show", "dev", "enp23s0f0np0"], text=True)
        if re.search(r"^qdisc tbf ", qdisc, re.M):
            raise RuntimeError("backend duplicates shaping")
        result["qdisc"] = qdisc
    return result


if __name__ == "__main__":
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("role", choices=("worker", "backend", "loader"))
    print(json.dumps(capture(p.parse_args().role)))
