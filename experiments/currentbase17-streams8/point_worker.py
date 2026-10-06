"""Readiness and idle-boundary counters for the dedicated relay, run on worker."""
import argparse
import json
import pathlib
import re
import time
import urllib.request

from launcher import checked_config, service_state
from runtime_probe import wait_idle


def read_http(endpoint, path, reset=False):
    request = urllib.request.Request("http://" + endpoint + path, method="POST" if reset else "GET")
    with urllib.request.urlopen(request, timeout=10) as r:
        if reset:
            if r.status != 204:
                raise RuntimeError("counter reset rejected")
            return {"reset": True}
        return json.load(r)


def inspect(config, action):
    c = checked_config(config)
    deadline = time.monotonic() + 180
    while True:
        service = service_state(c["unit"])
        if service.get("ActiveState") != "active":
            raise RuntimeError("owned relay not active: " + repr(service))
        pid = int(service["MainPID"])
        if pathlib.Path(f"/proc/{pid}/exe").resolve() != pathlib.Path(c["relay_binary"]).resolve():
            raise RuntimeError("running binary differs from configured version")
        launch = json.loads((pathlib.Path(c["log_dir"]) / "launch.json").read_text())
        if launch["config"] != c:
            raise RuntimeError("launch receipt/config differs")
        if action == "diagnostic":
            state = read_http(c["relay_endpoint"], "/__snapshare/runtime-state")
            stats = read_http(c["relay_endpoint"], "/__snapshare/remote-fetch-stats")
            return dict(service=service, runtime=state, fetch_stats=stats, settled=False,
                        diagnostic_only=True, captured_at=time.time())
        log = (pathlib.Path(c["log_dir"]) / "relay.log").read_text()
        sources_ready = c["security"] == "full" or ("Loaded chunk hashes for 17 images" in log and
                        re.search(r"Loaded rootfs chunk hashes, total [1-9][0-9]* chunks", log))
        if sources_ready:
            break
        if "level=error" in log or time.monotonic() >= deadline:
            raise RuntimeError("classification sources did not initialize; inspect owned relay log")
        time.sleep(1)
    state = wait_idle(c["relay_endpoint"], 180)
    cache = read_http(c["ws_cache_endpoint"], "/cached-working-sets")
    stats = read_http(c["relay_endpoint"], "/__snapshare/remote-fetch-stats", action == "reset")
    return dict(service=service, runtime=state, cache=cache, fetch_stats=stats,
                binary=c["relay_binary"], captured_at=time.time())


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("config", type=pathlib.Path)
    p.add_argument("action", choices=("ready", "reset", "final", "diagnostic"))
    a = p.parse_args()
    print(json.dumps(inspect(json.loads(a.config.read_text()), a.action)))


if __name__ == "__main__":
    main()
