#!/usr/bin/env python3
"""Own a fresh loop-backed pool for the fixed 10GB legacy snapshot profile.

The four-image budget covers the base image, source and two simultaneous clones.
Product setup's existing 100G pool is independent of this dedicated test helper.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import signal
import stat
import subprocess
import tomllib

GIB = 1024 ** 3
DATA_BYTES = 40 * GIB
METADATA_BYTES = GIB
MIN_FREE_BYTES = 43 * GIB
ROOT = Path(__file__).resolve().parents[1]


class System:
    def run(self, *args):
        return subprocess.check_output(args, text=True, stderr=subprocess.STDOUT,
                                       env=dict(os.environ, FICD_DM_VOLUME_GROUP=""))

    def mappings(self):
        return set(self.run("sudo", "-n", "dmsetup", "info", "-c", "--noheadings", "-o", "name").split())

    def loops(self):
        return json.loads(self.run("sudo", "-n", "losetup", "--list", "--json",
                                   "--output", "NAME,BACK-FILE")).get("loopdevices", [])

    def device(self, path):
        number = os.stat(path).st_rdev
        return f"{os.major(number)}:{os.minor(number)}"


def capacity_plan(config, available):
    settings = tomllib.loads(Path(config).read_text())
    base = settings["plugins"]["io.containerd.snapshotter.v1.devmapper"]["base_image_size"]
    # containerd uses units.RAMInBytes: its shipped "10GB" means 10 GiB.
    if base != "10GB":
        raise ValueError("review the four-image test-pool budget when changing the 10GB profile")
    if available < MIN_FREE_BYTES:
        raise ValueError("legacy snapshot pool requires at least 43 GiB available host storage")
    return {"base_image_size": base, "base_image_bytes": 10 * GIB,
            "image_budget": 4, "simultaneous_vms": 3,
            "data_bytes": DATA_BYTES, "metadata_bytes": METADATA_BYTES,
            "minimum_available_bytes": MIN_FREE_BYTES, "observed_available_bytes": available}


def identity(path):
    info = path.lstat()
    if not stat.S_ISREG(info.st_mode) or info.st_nlink != 1:
        raise ValueError(f"backing is not an exclusively owned regular file: {path}")
    return {"device": info.st_dev, "inode": info.st_ino, "size": info.st_size,
            "uid": info.st_uid, "gid": info.st_gid, "mode": stat.S_IMODE(info.st_mode)}


def associated(loops, directory):
    return [loop for loop in loops if loop.get("back-file", "").removesuffix(" (deleted)")
            in {str(directory / "data"), str(directory / "metadata")}]


def save(path, value):
    temporary = path.with_suffix(".tmp")
    with temporary.open("w") as output:
        json.dump(value, output, indent=2)
        output.write("\n")
        output.flush()
        os.fsync(output.fileno())
    temporary.replace(path)


def geometry(table, status_text, plan):
    rows = table.strip().splitlines()
    statuses = status_text.strip().splitlines()
    if len(rows) != 1 or len(statuses) != 1:
        raise ValueError("expected exactly one thin-pool table/status row")
    fields, status_fields = rows[0].split(), statuses[0].split()
    if len(fields) < 7 or fields[0] != "0" or fields[2] != "thin-pool":
        raise ValueError("unexpected thin-pool table")
    if len(status_fields) < 8 or status_fields[0] != "0" or status_fields[2] != "thin-pool":
        raise ValueError("unexpected thin-pool status")
    capacity = int(fields[1]) * 512
    chunk_bytes = int(fields[5]) * 512
    used, total = map(int, status_fields[5].split("/"))
    if capacity != plan["data_bytes"] or total * chunk_bytes != capacity:
        raise ValueError("actual thin-pool capacity differs from the four-image budget")
    if status_fields[7] != "rw" or (total - used) * chunk_bytes < 3 * plan["base_image_bytes"]:
        raise ValueError("thin-pool is not writable or lacks source plus two-clone capacity")
    return {"data_bytes": capacity, "chunk_bytes": chunk_bytes,
            "used_data_bytes": used * chunk_bytes, "available_data_bytes": (total - used) * chunk_bytes,
            "metadata_device": fields[3], "data_device": fields[4], "mode": status_fields[7]}


def checked_owner(receipt):
    owner = json.loads(receipt.read_text())
    source, pool = Path(owner["runtime_source"]), owner["pool"]
    if not source.is_absolute() or source.resolve() != source or not re.fullmatch(r"[A-Za-z0-9][A-Za-z0-9_-]{0,62}", pool):
        raise ValueError("invalid pool ownership receipt")
    directory = source / "tmp/devmapper" / pool
    if owner["directory"] != str(directory):
        raise ValueError("ownership directory differs from pool identity")
    for name, expected in owner["backings"].items():
        path = directory / name
        if name not in {"data", "metadata"} or ((path.exists() or path.is_symlink()) and identity(path) != expected):
            raise ValueError("backing identity changed; refuse pool cleanup")
    helper = source / "tools/thinpool.sh"
    if hashlib.sha256(helper.read_bytes()).hexdigest() != owner["runtime_helper_sha256"]:
        raise ValueError("runtime pool helper changed; refuse cleanup")
    return owner, source, directory


def remove(receipt, system=None):
    system = system or System()
    initial = json.loads(receipt.read_text())
    if initial.get("removed"):
        return initial
    owner, source, directory = checked_owner(receipt)
    pool = owner["pool"]
    loops = associated(system.loops(), directory)
    for loop in loops:
        name = Path(loop["back-file"].removesuffix(" (deleted)")).name
        if name not in owner["backings"] or not (directory / name).exists():
            raise ValueError("loop backing identity is unverified; retain resources")
    devices = {system.device(loop["name"]) for loop in loops}
    mappings = system.mappings()
    pool_device = system.device("/dev/mapper/" + pool) if pool in mappings else None
    # Do not detach a backing referenced by any mapping outside this private pool.
    tables = system.run("sudo", "-n", "dmsetup", "table", "--noheadings")
    for row in tables.splitlines():
        name, _, fields = row.partition(":")
        if name == pool:
            continue
        tokens = fields.split()
        if name.startswith(pool + "-snap-"):
            if len(tokens) < 4 or tokens[2] != "thin" or tokens[3] != pool_device:
                raise ValueError("snapshot name refers to another device; retain resources")
            continue
        if (devices | {pool_device}).intersection(tokens):
            raise ValueError("owned backing is referenced by another mapping; retain resources")
    if pool in mappings:
        table = system.run("sudo", "-n", "dmsetup", "table", pool)
        fields = table.split()
        if len(fields) < 7 or fields[2] != "thin-pool" or not {fields[3], fields[4]}.issubset(devices):
            raise ValueError("pool does not map the owned backing devices; refuse cleanup")
        system.run("bash", str(source / "tools/thinpool.sh"), "remove", pool)
    else:
        if any(name.startswith(pool + "-snap-") for name in mappings):
            raise ValueError("snapshot mapping remains without its pool; retain resources")
        # Partial create may have attached loops before dmsetup create failed.
        for loop in loops:
            system.run("sudo", "-n", "losetup", "--detach", loop["name"])
    if pool in system.mappings() or associated(system.loops(), directory):
        raise ValueError("owned pool/loop cleanup did not finish")
    for name, expected in owner["backings"].items():
        path = directory / name
        if path.exists():
            if identity(path) != expected:
                raise ValueError("backing changed during cleanup; retain it")
            path.unlink()
    owner["removed"] = True
    save(receipt, owner)
    return owner


def create(source, pool, evidence, system=None):
    system = system or System()
    source = Path(source).resolve(strict=True)
    if not re.fullmatch(r"[A-Za-z0-9][A-Za-z0-9_-]{0,62}", pool):
        raise ValueError("invalid private pool name")
    evidence = Path(evidence).resolve()
    if evidence.is_relative_to(source):
        raise ValueError("pool evidence must be outside the runtime checkout")
    evidence.mkdir(mode=0o700)
    directory = source / "tmp/devmapper" / pool
    if directory.exists() or directory.is_symlink() or pool in system.mappings():
        raise ValueError("private pool path or mapper already exists")
    if associated(system.loops(), directory):
        raise ValueError("private backing already has an associated loop")
    usage = os.statvfs(source)
    plan = capacity_plan(ROOT / "configs/firecracker-containerd/config.toml", usage.f_bavail * usage.f_frsize)
    save(evidence / "capacity-plan.json", plan)
    for parent in [source / "tmp", source / "tmp/devmapper"]:
        if parent.is_symlink():
            raise ValueError("private backing parent must not be a symlink")
        parent.mkdir(mode=0o700, exist_ok=True)
    directory.mkdir(mode=0o700)
    receipt = evidence / "ownership.json"
    owner = {"runtime_source": str(source), "pool": pool, "directory": str(directory),
             "plan": plan, "backings": {}, "removed": False,
             "runtime_helper_sha256": hashlib.sha256((source / "tools/thinpool.sh").read_bytes()).hexdigest()}
    save(receipt, owner)
    try:
        for name, size in [("data", DATA_BYTES), ("metadata", METADATA_BYTES)]:
            fd = os.open(directory / name, os.O_CREAT | os.O_EXCL | os.O_WRONLY | os.O_NOFOLLOW, 0o600)
            try:
                owner["backings"][name] = identity(directory / name)
                save(receipt, owner)
                os.ftruncate(fd, size)
            finally:
                os.close(fd)
                owner["backings"][name] = identity(directory / name)
                save(receipt, owner)
        output = system.run("bash", str(source / "tools/thinpool.sh"), "create", pool)
        (evidence / "create.log").write_text(output)
        table = system.run("sudo", "-n", "dmsetup", "table", pool)
        status_text = system.run("sudo", "-n", "dmsetup", "status", pool)
        (evidence / "table.txt").write_text(table)
        (evidence / "status.txt").write_text(status_text)
        observed = geometry(table, status_text, plan)
        loops = associated(system.loops(), directory)
        by_backing = {loop["back-file"]: system.device(loop["name"]) for loop in loops}
        if observed["data_device"] != by_backing.get(str(directory / "data")) or observed["metadata_device"] != by_backing.get(str(directory / "metadata")):
            raise ValueError("actual pool uses different backing devices")
        save(evidence / "capacity-verified.json", observed)
        return owner
    except BaseException as error:
        (evidence / "create-failure.txt").write_text(str(error) + "\n" + str(getattr(error, "output", "")))
        try:
            remove(receipt, system)
        except BaseException as cleanup_error:
            (evidence / "cleanup-failure.txt").write_text(str(cleanup_error) + "\n")
        raise


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    commands = parser.add_subparsers(dest="command", required=True)
    provision = commands.add_parser("create")
    provision.add_argument("--runtime-source", type=Path, required=True)
    provision.add_argument("--pool", required=True)
    provision.add_argument("--evidence", type=Path, required=True)
    cleanup = commands.add_parser("remove")
    cleanup.add_argument("--receipt", type=Path, required=True)
    args = parser.parse_args()
    def interrupted(_signum, _frame):
        raise InterruptedError("pool helper interrupted")
    signal.signal(signal.SIGTERM, interrupted)
    result = create(args.runtime_source, args.pool, args.evidence) if args.command == "create" else remove(args.receipt)
    print(json.dumps(result, indent=2))


if __name__ == "__main__":
    main()
