"""Retire a failed owned relay with stale VM bookkeeping; never accept its sample.

Unlike normal launcher.stop, this diagnostic-only path accepts a nonempty active
VM list only when those exact processes and all writers are already absent.
It does not delete networks, cache, results, or restart any unit.
"""
import argparse
import json
import pathlib
import subprocess
import time

from launcher import checked_config, owned_vm_ids, remaining_vm_processes, service_state
from point_worker import inspect


def retire(config, expected_pid, stale_vm, output):
    c = checked_config(json.loads(config.read_text()))
    before_path = output.with_suffix(".before.json")
    if before_path.exists():
        before = json.loads(before_path.read_text())
        if before["config"] != c or before["pid"] != expected_pid or before["stale_vm"] != stale_vm:
            raise ValueError("retirement receipt identity differs")
    else:
        diagnostic = inspect(c, "diagnostic")
        state = diagnostic["runtime"]
        if int(diagnostic["service"]["MainPID"]) != expected_pid:
            raise ValueError("not the explicitly identified failed relay")
        if state["active_vm_ids"] != [stale_vm] or any(state[k] for k in (
                "active_requests", "active_uffd_handlers", "cleanup_tasks",
                "pending_shim_refills", "pending_ws_cache_writes")):
            raise ValueError("writers active or stale VM identity differs")
        ids = owned_vm_ids((pathlib.Path(c["log_dir"])/"relay.log").read_text(), expected_pid)
        if stale_vm not in ids or remaining_vm_processes([stale_vm]):
            raise ValueError("stale VM is foreign or still has a running process")
        before = dict(config=c, pid=expected_pid, stale_vm=stale_vm, vm_ids=ids,
                      diagnostic=diagnostic, captured_at=time.time(), diagnostic_only=True)
        with before_path.open("x") as f:
            json.dump(before, f, indent=2)
    service = service_state(c["unit"])
    if service.get("ActiveState") == "active":
        if int(service["MainPID"]) != expected_pid:
            raise ValueError("failed relay PID changed")
        subprocess.run(["sudo", "-n", "systemctl", "stop", c["unit"]], check=True)
    service = service_state(c["unit"])
    remaining = remaining_vm_processes(before["vm_ids"])
    result = dict(unit=c["unit"], binary=c["relay_binary"], service=service,
                  vm_ids=before["vm_ids"], remaining_processes=remaining,
                  captured_at=time.time(), diagnostic_only=True,
                  note="Failed sample stays invalid; this is not normal shutdown acceptance. Networks/cache are untouched.")
    with output.open("x") as f:
        json.dump(result, f, indent=2)
    if service.get("ActiveState") in ("active", "activating", "deactivating") or remaining:
        raise RuntimeError("retirement incomplete; inspect saved evidence, do not replay")
    print(json.dumps(result))


if __name__ == "__main__":
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("config", type=pathlib.Path)
    p.add_argument("--expected-pid", type=int, required=True)
    p.add_argument("--stale-vm", required=True)
    p.add_argument("--output", type=pathlib.Path, required=True)
    a = p.parse_args()
    retire(a.config, a.expected_pid, a.stale_vm, a.output)
