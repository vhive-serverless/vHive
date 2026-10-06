"""Audit the real missing-required-WS failure, including client and relay evidence."""
import argparse
import json
import pathlib
import re

from validate_point import messages


def audit_negative(result, invocation, log, state):
    if result["returncode"] == 0 or result["success"]:
        raise ValueError("missing WS must fail the client invocation")
    if not re.search(r"Snapshot Load Error|unexpected HTTP status code received from server: 500", invocation):
        raise ValueError("client did not receive a server failure")
    rows = messages(log)
    causes = [msg for _, msg in rows if msg.startswith("LoadSnapshot error is ")]
    if len(causes) != 1 or not all(term in causes[0] for term in (
            "loading required split WS content", "stream-decode compressed working set", "specified key does not exist")):
        raise ValueError("failure is not the intentionally absent required WS payload")
    for line, msg in rows:
        if (msg.startswith(("ZSTD_WS_DECODE ", "Invocation to ", "Starting handler at ", "created VM with ID ")) or
                "starting from base snapshot" in msg or "starting from image" in msg):
            raise ValueError("failure fell through into restore/execution")
        if "level=error" in line and msg != causes[0]:
            raise ValueError("unexpected secondary failure: " + line)
    acquired = re.findall(r'Orchestrator received LoadSnapshot[^\n]*vmID=([^\s"]+)', log)
    if len(acquired) != 1 or not any("Successfully removed shim" in msg and "vmID=" + acquired[0] in line for line, msg in rows):
        raise ValueError("failed restore shim not released")
    if state["active_vm_ids"] != [] or any(state[k] != 0 for k in (
            "active_requests", "active_uffd_handlers", "cleanup_tasks", "pending_shim_refills", "pending_ws_cache_writes")):
        raise ValueError("runtime has not settled")
    return dict(purpose="real missing-required-WS negative gate", accepted=True, cause=causes[0],
                released_vm=acquired[0], function_invocations=0, uffd_handlers_started=0,
                note="Client reports HTTP 500 without the text body; relay log independently identifies the WS cause. Earlier wrapper expected_failure flag is not the verdict.")


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("root", type=pathlib.Path)
    a = p.parse_args()
    result = audit_negative(json.loads((a.root / "invocation-result.json").read_text()),
                            (a.root / "invocation.log").read_text(), (a.root / "relay.log").read_text(),
                            json.loads((a.root / "runtime-state-final.json").read_text()))
    with (a.root / "negative-validation.json").open("x") as f:
        json.dump(result, f, indent=2)
        f.write("\n")
    print(json.dumps(result))


if __name__ == "__main__":
    main()
