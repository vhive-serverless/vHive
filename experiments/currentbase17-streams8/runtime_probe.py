"""Observe the dedicated relay until requests and their asynchronous work finish."""
import argparse
import json
import pathlib
import time
import urllib.error
import urllib.request


def idle(state):
    return (state["active_vm_ids"] == [] and all(state[k] == 0 for k in (
        "active_requests", "cleanup_tasks", "pending_shim_refills", "pending_ws_cache_writes", "active_uffd_handlers")))


def wait_idle(endpoint, timeout=120):
    deadline = time.monotonic() + timeout
    last = None
    while time.monotonic() < deadline:
        try:
            with urllib.request.urlopen("http://" + endpoint + "/__snapshare/runtime-state", timeout=10) as r:
                last = json.load(r)
            if idle(last):
                return last
        except (urllib.error.URLError, TimeoutError) as e:
            last = str(e)
        time.sleep(0.25)
    raise RuntimeError("runtime did not settle; observe existing process, do not restart: " + repr(last))


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("--endpoint", required=True)
    p.add_argument("--timeout", type=float, default=120)
    p.add_argument("--output", type=pathlib.Path, required=True)
    a = p.parse_args()
    if a.output.exists():
        raise FileExistsError(a.output)
    state = wait_idle(a.endpoint, a.timeout)
    with a.output.open("x") as f:
        json.dump(state, f, indent=2)
        f.write("\n")
    print("RUNTIME_IDLE", json.dumps(state))


if __name__ == "__main__":
    main()
