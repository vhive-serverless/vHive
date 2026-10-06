"""Worker-only lifecycle gate: close one incomplete RPC after real VM restoration.

No corpus edits or fault-injection flags in the relay. The incomplete body keeps
the RPC pending; cancellation occurs only after the relay reports this VM loaded.
This is failure/lifetime evidence, never a successful invocation/performance sample.
"""
import argparse
import json
import pathlib
import re
import socket
import struct
import time

from point_worker import inspect
from validate_point import messages


def run(config, plan, output):
    c = json.loads(config.read_text())
    if c["security"] != "partial" or not c["ws_coalescing"] or c["mode"] != "remote":
        raise ValueError("requires the owned remote SplitSnap AES gate")
    w = next(w for w in json.loads(plan.read_text())["workloads"]["workloads"]
             if w["profile"] == "aes-go-45000-45450")
    ready = inspect(c, "ready")
    output.mkdir(parents=True, exist_ok=False)
    (output/"ready.json").write_text(json.dumps(ready, indent=2))
    log_path = pathlib.Path(c["log_dir"])/"relay.log"
    offset = log_path.stat().st_size
    revision = w["snapshot"] + "-cancelgate" + str(time.time_ns()) + "-00000"
    headers = {"Host": c["relay_endpoint"], "Content-Type": "application/grpc", "TE": "trailers",
               "Content-Length": "1024", "image": w["image"], "revision": revision,
               "functionPort": str(w["function_port"])}
    request = "POST /aes.Aes/ShowEncryption HTTP/1.1\r\n" + "".join(
        f"{key}: {value}\r\n" for key, value in headers.items()) + "\r\n"
    host, port = c["relay_endpoint"].rsplit(":", 1)
    event = dict(purpose="cancel pending RPC after real LoadSnapshot/UFFD connection", revision=revision,
                 binary=c["relay_binary"], unit=c["unit"], started_at=time.time(),
                 incomplete_grpc_body=True, declared_body_bytes=1024, sent_body_bytes=5)
    vm = None
    with socket.create_connection((host, int(port)), timeout=10) as connection:
        connection.sendall(request.encode() + b"\0" + struct.pack(">I", 1019))
        deadline = time.monotonic() + 60
        with log_path.open() as log:
            log.seek(offset)
            while time.monotonic() < deadline:
                line = log.readline()
                match = re.search(r"created VM with ID (\S+) and IP \S+ for revision " + re.escape(revision), line)
                if match:
                    vm = match[1]
                    event.update(vm_id=vm, restored_event=line.strip(), cancelled_at=time.time())
                    connection.shutdown(socket.SHUT_RDWR)
                    break
                if "LoadSnapshot error is" in line:
                    raise RuntimeError("restore failed before the intended cancellation: " + line)
                if not line:
                    time.sleep(.005)
    (output/"cancellation.json").write_text(json.dumps(event, indent=2))
    if vm is None:
        raise RuntimeError("no confirmed loaded VM before observation deadline; do not relabel as late cancellation")
    try:
        final = inspect(c, "final")
        (output/"final.json").write_text(json.dumps(final, indent=2))
    finally:
        with log_path.open() as log:
            log.seek(offset)
            text = log.read()
        (output/"relay.log").write_text(text)
    rows = messages(text)
    required = (
        any(msg.startswith(f"VM_TERMINATION_CONFIRMED vm_id={vm} ") for _, msg in rows),
        any(msg == "Stopped VM successfully" and f"vmID={vm}" in line for line, msg in rows),
        any(msg.startswith("Handled ") and f"/tmp/{vm}.uffd.sock" in line for line, msg in rows),
        "context canceled" in text,
    )
    if not all(required):
        raise ValueError("missing cancellation/VM/UFFD cleanup evidence: " + repr(required))
    (output/"validation.json").write_text(json.dumps(dict(accepted=True, vm_id=vm,
        binary=c["relay_binary"], cancelled_after_load=True, uffd_released=True,
        runtime_idle=True, performance_sample=False), indent=2))
    print(json.dumps(dict(accepted=True, vm_id=vm, purpose=event["purpose"])))


if __name__ == "__main__":
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("--config", type=pathlib.Path, required=True)
    p.add_argument("--plan", type=pathlib.Path, required=True)
    p.add_argument("--output", type=pathlib.Path, required=True)
    a = p.parse_args()
    run(a.config, a.plan, a.output)
