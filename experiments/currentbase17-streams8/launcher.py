#!/usr/bin/env python3
"""Manage only one explicitly configured eight-stream relay, not its shared services.

Run on the worker: launcher.py CONFIG.json start|status|stop|command.
The config's run_root owns the cache/scratch/logs; Firecracker-containerd,
demux, resolver, images, MongoDB, MinIO and the old relay are never stopped.
"""
import argparse
import json
import pathlib
import re
import socket
import subprocess
import time


def checked_config(config):
    c = dict(config)
    for key in ("run_id", "point_id"):
        if not re.fullmatch(r"[a-z0-9][a-z0-9_-]{0,100}", c[key]):
            raise ValueError(f"invalid {key}")
    root = pathlib.Path(c["run_root"])
    if not root.is_absolute() or "streams8" not in root.parts or ".." in root.parts:
        raise ValueError("run_root must be an absolute task-specific streams8 directory")
    for key in ("cache_dir", "scratch_dir", "log_dir"):
        path = pathlib.Path(c[key])
        if not path.is_absolute() or root not in path.parents or ".." in path.parts:
            raise ValueError(f"{key} must be inside run_root")
    cache, scratch = pathlib.Path(c["cache_dir"]), pathlib.Path(c["scratch_dir"])
    if cache == scratch or cache in scratch.parents or scratch in cache.parents:
        raise ValueError("cache and shutdown-removed scratch directories must be disjoint")
    if not re.fullmatch(r"[a-z]{1,4}", c["network_name_prefix"]):
        raise ValueError("an independent 1-4 letter network_name_prefix is required")
    for key in ("relay_binary", "relay_cwd", "images_dir"):
        if not pathlib.Path(c[key]).is_absolute():
            raise ValueError(f"{key} must be absolute")
    for key in ("relay_endpoint", "ws_cache_endpoint"):
        host, port = c[key].rsplit(":", 1)
        socket.inet_aton(host)
        if not 1 <= int(port) <= 65535 or int(port) in (8080, 8081):
            raise ValueError("use independent ports, not legacy 8080/8081")
    if c["relay_endpoint"] == c["ws_cache_endpoint"]:
        raise ValueError("relay and inventory ports must differ")
    if c["security"] not in ("full", "partial", "no-image-sharing"):
        raise ValueError("formal matrix uses full/partial/no-image-sharing, including oracle=partial")
    if c["mode"] not in ("remote", "local"):
        raise ValueError("mode must be remote or local")
    if c["vm_mib"] not in (512, 2048, 3072) or c["chunk_size"] not in (4096, 131072):
        raise ValueError("unexpected formal memory/chunk size")
    if c["mode"] == "local" and c["clean"]:
        raise ValueError("all-local must retain its prepared cache")
    c["unit"] = f"splitsnap-streams8-{c['run_id']}-{c['point_id']}.service"
    return c


def relay_command(c):
    c = checked_config(c)
    args = [c["relay_binary"], "-ss=proxy", "-upf", "-ws", "-lazy", "-chunking",
            "-chunkCompression", "-zstdLevel=3", "-zstdWSLayout=streams8-v1",
            "-zstdFetchers=8", "-j=16", "-netPoolSize=1", "-cacheSize=1000000",
            "-dbg", "-freshSourceDelay=10s",
            f"-endpoint={c['relay_endpoint']}", f"-snapshots={c['mode']}",
            f"-wsCacheEndpoint={c['ws_cache_endpoint']}",
            f"-snapshotsDir={c['cache_dir']}", f"-snapshotsScratchDir={c['scratch_dir']}",
            f"-networkNamePrefix={c['network_name_prefix']}",
            f"-vethPrefix={c['veth_prefix']}", f"-clonePrefix={c['clone_prefix']}",
            f"-hostIface={c['host_iface']}", f"-dnsNameservers={c['dns']}",
            f"-vmMemSizeMib={c['vm_mib']}", f"-chunkSize={c['chunk_size']}",
            f"-security={c['security']}",
            f"-cacheSnaps={'true' if c['mode'] == 'local' else 'false'}",
            f"-minioCredentials={c['minio_endpoint']};minio;minio123"]
    if c["ws_coalescing"]:
        args += ["-wsCoalescing", "-wsCompression"]
    if c["base_snap"]:
        args.append("-baseSnap")
    if c["clean"]:
        args.append("-clean")
    return args


def service_state(unit):
    p = subprocess.run(["systemctl", "show", unit, "--property=LoadState,ActiveState,SubState,MainPID,Result"],
                       text=True, stdout=subprocess.PIPE, check=False)
    return dict(line.split("=", 1) for line in p.stdout.splitlines() if "=" in line)


def check_endpoint_available(endpoint):
    host, port = endpoint.rsplit(":", 1)
    with socket.socket() as s:
        # Match Go's TCP listener: an ended point may leave TIME_WAIT sockets.
        # SO_REUSEADDR permits those, not another active TCP listener; do not
        # enable SO_REUSEPORT. Listen also catches sockets only bound elsewhere.
        s.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
        s.bind((host, int(port)))
        s.listen(1)


def owned_vm_ids(log, pid):
    ids = sorted(set(re.findall(r"\bshim-[0-9]+-[0-9]+\b", log)))
    if not ids or any(not name.startswith(f"shim-{pid}-") for name in ids):
        raise ValueError("missing/foreign VM ownership in this relay log")
    return ids


def remaining_vm_processes(ids, proc=pathlib.Path("/proc")):
    ids = set(ids)
    remaining = []
    for entry in proc.iterdir():
        if not entry.name.isdigit():
            continue
        try:
            args = (entry / "cmdline").read_bytes().decode().split("\0")
        except (FileNotFoundError, ProcessLookupError):
            # procfs can report ESRCH while a task is concurrently reaped.
            continue
        if pathlib.Path(args[0]).name not in ("firecracker", "containerd-shim-aws-firecracker"):
            continue
        for i, arg in enumerate(args):
            vm = args[i+1] if arg in ("--id", "-id") and i+1 < len(args) else (
                arg.split("=", 1)[1] if arg.startswith(("--id=", "-id=")) else None)
            if vm in ids:
                remaining.append(dict(pid=int(entry.name), vm_id=vm))
                break
    return remaining


def stop(c):
    log_dir = pathlib.Path(c["log_dir"])
    receipt = json.loads((log_dir / "launch.json").read_text())
    if receipt["config"] != c:
        raise ValueError("launch receipt/config mismatch")
    before_file = log_dir / "shutdown-before.json"
    if not before_file.exists():
        from runtime_probe import wait_idle
        service = service_state(c["unit"])
        if service.get("ActiveState") != "active":
            raise RuntimeError("no pre-shutdown ownership receipt; inspect ended unit, do not replay")
        pid = int(service["MainPID"])
        if pathlib.Path(f"/proc/{pid}/exe").resolve() != pathlib.Path(c["relay_binary"]).resolve():
            raise ValueError("shutdown binary differs")
        runtime = wait_idle(c["relay_endpoint"], 180)
        ids = owned_vm_ids((log_dir / "relay.log").read_text(), pid)
        with before_file.open("x") as f:
            json.dump(dict(config=c, relay_pid=pid, vm_ids=ids, runtime=runtime), f, indent=2)
    before = json.loads(before_file.read_text())
    if before["config"] != c:
        raise ValueError("shutdown receipt/config mismatch")
    if service_state(c["unit"]).get("ActiveState") == "active":
        subprocess.run(["sudo", "-n", "systemctl", "stop", c["unit"]], check=True)
    service = service_state(c["unit"])
    remaining = remaining_vm_processes(before["vm_ids"])
    if (service.get("ActiveState") in ("active", "activating", "deactivating", "failed") or
            service.get("Result", "success") != "success" or remaining):
        raise RuntimeError(f"owned shutdown incomplete: service={service}, remaining={remaining}")
    result = dict(unit=c["unit"], binary=c["relay_binary"], service=service,
                  vm_ids=before["vm_ids"], remaining_processes=remaining,
                  processes_absent=True, captured_at=time.time())
    target = log_dir / "shutdown.json"
    if not target.exists():
        with target.open("x") as f:
            json.dump(result, f, indent=2)
    return result


def start(c):
    args = relay_command(c)
    if service_state(c["unit"]).get("LoadState") != "not-found":
        raise RuntimeError("unit already exists; inspect it, do not replay start")
    for key in ("relay_endpoint", "ws_cache_endpoint"):
        check_endpoint_available(c[key])
    for asset in (c["relay_binary"], c["relay_cwd"], c["images_dir"],
                  "/run/firecracker-containerd/containerd.sock",
                  "/var/lib/demux-snapshotter/snapshotter.sock"):
        if not pathlib.Path(asset).exists():
            raise FileNotFoundError(asset)
    for key in ("cache_dir", "scratch_dir", "log_dir"):
        pathlib.Path(c[key]).mkdir(parents=True, exist_ok=True)
    # SnapshotManager finds immutable classification assets beside its cache.
    image_link = pathlib.Path(c["cache_dir"]).parent / "images"
    if image_link.exists() or image_link.is_symlink():
        if image_link.resolve() != pathlib.Path(c["images_dir"]).resolve():
            raise ValueError("existing images link does not match configured assets")
    else:
        image_link.symlink_to(c["images_dir"], target_is_directory=True)
    log_dir = pathlib.Path(c["log_dir"])
    with (log_dir / "launch.json").open("x") as f:
        json.dump({"config": c, "argv": args}, f, indent=2)
        f.write("\n")
    subprocess.run(["sudo", "-n", "systemd-run", f"--unit={c['unit']}", "--service-type=exec",
                    f"--property=WorkingDirectory={c['relay_cwd']}",
                    f"--property=StandardOutput=append:{log_dir / 'relay.log'}",
                    f"--property=StandardError=append:{log_dir / 'relay.log'}",
                    "--property=KillMode=mixed", "--property=TimeoutStopSec=90", "--", *args], check=True)
    print(json.dumps(service_state(c["unit"])))


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("config", type=pathlib.Path)
    p.add_argument("action", choices=("command", "start", "status", "stop"))
    a = p.parse_args()
    c = checked_config(json.loads(a.config.read_text()))
    if a.action == "command":
        print(json.dumps(relay_command(c)))
    elif a.action == "status":
        print(json.dumps(service_state(c["unit"])))
    elif a.action == "start":
        start(c)
    else:
        print(json.dumps(stop(c)))


if __name__ == "__main__":
    main()
