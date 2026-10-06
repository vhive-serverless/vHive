#!/usr/bin/env python3
"""Start new MinIO stores from a frozen plan; never stop or reuse an old store.

Run as root on the backend after the read-only source inventory is complete.
"""
import argparse
import json
import pathlib
import shutil
import socket
import subprocess
import time
import urllib.request

IMAGE = "quay.io/minio/minio:RELEASE.2024-12-18T13-15-44Z"


def start(plan, inventory, selected, capacity):
    planned = {j["corpus_id"]: j for j in plan["jobs"]}
    scanned = {j["corpus_id"]: j for j in inventory["jobs"]}
    if planned.keys() != scanned.keys():
        raise ValueError("plan/inventory stores differ")
    root = pathlib.Path("/mnt/snapshare-zstd-streaming/streams8")/plan["run_id"]
    # Reserve new WS and at least the largest 60-alias batch as well as copies.
    # All-alias publication still requires its own check with actual new sizes.
    source_bytes = sum(j["bytes"] for j in scanned.values())
    source_objects = sum(len(j["objects"]) for j in scanned.values())
    if capacity["source_bytes"] != source_bytes or capacity["source_objects"] != source_objects:
        raise ValueError("capacity report/inventory mismatch")
    required = capacity["staged_required_bytes"]
    free = shutil.disk_usage(root.parent.parent).free
    if free < required:
        raise RuntimeError(f"insufficient corpus disk headroom: free={free}, required={required}")
    for key, j in planned.items():
        if selected and key != selected:
            continue
        if pathlib.Path(j["data_dir"]).parent != root or not j["container"].startswith("snapshare-streams8-"+plan["run_id"]+"-"):
            raise ValueError("unexpected data directory/container ownership")
        if (j["source"], j["destination"], j["prefixes"]) != (scanned[key]["source"], scanned[key]["destination"], scanned[key]["prefixes"]):
            raise ValueError("inventory does not match plan")
        check = subprocess.run(["docker", "inspect", j["container"]], stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
        if check.returncode == 0:
            existing = json.loads(check.stdout)[0]
            if existing["Config"]["Labels"].get("org.splitsnap.streams8.run") != plan["run_id"] or not existing["State"]["Running"]:
                raise RuntimeError("existing container not owned/running; inspect manually")
            mounts = {m["Destination"]: m["Source"] for m in existing["Mounts"]}
            if mounts.get("/data") != j["data_dir"] or existing["Config"]["Image"] != IMAGE:
                raise RuntimeError("existing corpus image/mount mismatch")
        else:
            host, port = j["destination"].rsplit(":", 1)
            with socket.socket() as s:
                s.bind((host, int(port)))
            path = pathlib.Path(j["data_dir"])
            path.parent.mkdir(parents=True, exist_ok=True)
            path.mkdir(exist_ok=False)
            subprocess.run(["docker", "run", "-d", "--name", j["container"],
                            "--label", "org.splitsnap.streams8.run="+plan["run_id"],
                            "-p", j["destination"]+":9000", "-e", "MINIO_ROOT_USER=minio",
                            "-e", "MINIO_ROOT_PASSWORD=minio123", "-v", j["data_dir"]+":/data",
                            IMAGE, "server", "/data"], check=True)
        ready = False
        for _ in range(30):
            try:
                with urllib.request.urlopen("http://"+j["destination"]+"/minio/health/ready", timeout=2) as response:
                    ready = response.status == 200
            except OSError:
                pass
            if ready:
                break
            time.sleep(1)
        if not ready:
            raise RuntimeError("MinIO did not become ready: "+key)
        print("CORPUS_SERVICE_READY", key, j["destination"], flush=True)


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("--plan", required=True, type=pathlib.Path)
    p.add_argument("--inventory", required=True, type=pathlib.Path)
    p.add_argument("--capacity", required=True, type=pathlib.Path)
    p.add_argument("--only", default="")
    a = p.parse_args()
    plan = json.loads(a.plan.read_text())
    if a.only and not any(j["corpus_id"] == a.only for j in plan["jobs"]):
        raise ValueError("unknown corpus")
    start(plan, json.loads(a.inventory.read_text()), a.only, json.loads(a.capacity.read_text()))


if __name__ == "__main__":
    main()
