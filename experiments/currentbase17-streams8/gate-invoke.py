#!/usr/bin/env python3
"""One real direct invocation on the loader. Functional evidence, not a latency run."""
import argparse
import json
import pathlib
import re
import subprocess
import time
import urllib.request


def invocation_args(plan, requests, profile, endpoint, token, invoker):
    w = next(w for w in plan["workloads"]["workloads"] if w["profile"] == profile)
    r = next(r for r in requests["requests"] if r["profile"] == profile)
    revision = f"{w['snapshot']}-{token}-00000"
    args = [invoker, "--address", endpoint, "--function-name", r["function_name"],
            "--image", w["image"], "--revision", revision,
            "--args", w.get("function_args", "").replace("__BACKEND_PRIVATE_IP__", plan["backend"]),
            "--env", w.get("function_env", ""), "--function-port", str(w["function_port"]),
            "--generator", r["generator"], "--value", r.get("value", ""),
            "--function-method", r.get("function_method", "default"),
            "--lower-bound", str(r.get("lower_bound", 1)), "--upper-bound", str(r.get("upper_bound", 10)),
            "--seed", str(r.get("seed", requests["seed"])), "--sequence-index", "0", "--timeout", "900s"]
    return args, r.get("reply_must_contain", "")


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("--plan", type=pathlib.Path, required=True)
    p.add_argument("--requests", type=pathlib.Path, required=True)
    p.add_argument("--profile", required=True)
    p.add_argument("--endpoint", required=True)
    p.add_argument("--output", type=pathlib.Path, required=True)
    p.add_argument("--invoker", default="/users/Liquidz/figure9-16/bin/direct-invoker")
    p.add_argument("--missing-ws-revision", default="", help="explicit deliberately invalid streams8 revision; require a top-level failure")
    a = p.parse_args()
    a.output.mkdir(parents=True, exist_ok=False)
    plan = json.loads(a.plan.read_text())
    args, required = invocation_args(plan, json.loads(a.requests.read_text()), a.profile,
                                     a.endpoint, "streamgate" + str(time.time_ns()), a.invoker)
    if a.missing_ws_revision:
        snapshot = next(w["snapshot"] for w in plan["workloads"]["workloads"] if w["profile"] == a.profile)
        if a.profile != "aes-go-45000-45450" or a.missing_ws_revision != snapshot + "-streams8-missingws-" + plan["run_id"]:
            raise ValueError("only the isolated negative AES fixture is accepted")
        args[args.index("--revision") + 1] = a.missing_ws_revision + "-streamgate" + str(time.time_ns()) + "-00000"
    (a.output / "argv.json").write_text(json.dumps(args, indent=2) + "\n")
    url = "http://" + a.endpoint + "/__snapshare/remote-fetch-stats"
    with urllib.request.urlopen(urllib.request.Request(url, method="POST"), timeout=10) as r:
        if r.status != 204:
            raise RuntimeError("counter reset failed")
    with (a.output / "invocation.log").open("w") as f:
        result = subprocess.run(args, stdout=f, stderr=subprocess.STDOUT)
    output = (a.output / "invocation.log").read_text()
    ok = result.returncode == 0 and bool(re.search(r"DIRECT_REQUEST_NS=[1-9][0-9]*", output))
    ok = ok and not re.search(r"Error:|Failed|Server Error|direct invocation failed", output)
    ok = ok and (not required or required in output)
    # The gRPC client reports the HTTP status but not the relay's text/plain
    # body. The saved relay log must independently prove the specific WS cause.
    expected_failure = bool(a.missing_ws_revision) and result.returncode != 0 and bool(
        re.search(r"Snapshot Load Error|unexpected HTTP status code received from server: 500", output))
    with urllib.request.urlopen(url, timeout=10) as r:
        stats = json.load(r)
    (a.output / "remote-fetch-stats.json").write_text(json.dumps(stats, indent=2) + "\n")
    (a.output / "invocation-result.json").write_text(json.dumps(dict(
        success=ok, expected_failure=expected_failure, returncode=result.returncode, purpose="negative missing-WS gate" if a.missing_ws_revision else "functional gate, not performance",
        pending="relay marker/8-stream/UFFD/release audit"), indent=2) + "\n")
    if a.missing_ws_revision:
        if not expected_failure:
            raise RuntimeError("required missing-WS failure did not reach the invoker")
        print("GATE_EXPECTED_FAILURE", a.profile, "relay missing-WS cause and no invocation still require audit")
    elif not ok:
        raise RuntimeError("invocation failed; retained output: " + str(a.output))
    else:
        print("GATE_INVOCATION_OK", a.profile, "relay audit still required")


if __name__ == "__main__":
    main()
