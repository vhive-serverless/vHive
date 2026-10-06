#!/usr/bin/env python3
"""Backend-local preparation controller; never invokes legacy service cleanup.

This prepares copies/transcodes only. It does not publish SYSTEMS_COMPLETE or
start the formal matrix: view validation, actual inventory and aliases gate it.
"""
import argparse
import fcntl
import json
import pathlib
import subprocess
import time


def publish(path, value):
    if path.exists():
        raise FileExistsError(path)
    tmp = path.with_suffix(path.suffix + ".tmp")
    with tmp.open("x") as f:
        json.dump(value, f, indent=2)
        f.write("\n")
    tmp.rename(path)


def execute(root, name, argv):
    stages = root / "stages"
    stages.mkdir(exist_ok=True)
    receipt = stages / (name + ".json")
    if receipt.exists():
        prior = json.loads(receipt.read_text())
        if prior["argv"] != argv or prior["rc"] != 0:
            raise RuntimeError("stage receipt/config mismatch: " + name)
        print("STAGE_RECORDED", name, flush=True)
        return
    started = time.time()
    print("STAGE_BEGIN", name, flush=True)
    with (stages / (name + ".log")).open("a") as output:
        subprocess.run(argv, stdout=output, stderr=subprocess.STDOUT, check=True)
    publish(receipt, dict(argv=argv, rc=0, started=started, ended=time.time()))
    print("STAGE_COMPLETE", name, flush=True)


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("run_root", type=pathlib.Path)
    a = p.parse_args()
    root = a.run_root
    if not root.is_absolute() or root.parent != pathlib.Path("/users/Liquidz/streams8"):
        raise ValueError("explicit backend streams8 run root required")
    plan_path, inventory = root / "config/plan.json", root / "source-inventory.json"
    plan = json.loads(plan_path.read_text())
    if plan["run_id"] != root.name or len(plan["jobs"]) != 18:
        raise ValueError("wrong plan")
    lock = (root / "prepare.lock").open("a")
    fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
    execute(root, "start-corpora", ["sudo", "-n", "python3", str(root / "config/start-corpora.py"),
                                   "--plan", str(plan_path), "--inventory", str(inventory),
                                   "--capacity", str(root / "capacity.json")])
    # Complete the small and large SplitSnap stores early for real UFFD gates.
    # No measurement may overlap this preparation controller's background I/O.
    jobs = sorted(plan["jobs"], key=lambda j: (j["kind"] != "partial-4k", j["tier"], j["kind"]))
    for j in jobs:
        name = j["corpus_id"]
        execute(root, "copy-" + name, [str(root / "bin/copy-corpus-streams8"),
                "-plan", str(plan_path), "-inventory", str(inventory), "-copy", "-only", name])
        if j["transcode_mode"] in ("full", "private"):
            report = root / "stages" / ("transcode-" + name + ".report.json")
            argv = [str(root / "bin/transcode-ws-streams8"), "-source", j["source"],
                    "-destination", j["destination"], "-mode", j["transcode_mode"],
                    "-workloads", str(root / "config" / (name + ".workloads.json")),
                    "-report", str(report)]
            # A final tool report may precede the controller receipt if interrupted.
            # Do not automatically bypass the tool's report-exists guard in that case.
            execute(root, "transcode-" + name, argv)
    publish(root / "COPIES_TRANSCODES_COMPLETE.json", dict(run_id=plan["run_id"],
            copies=18, transcode_jobs=9, layout="streams8-v1",
            pending=["oracle-view-copy-and-validation", "actual-layout-inventory", "cold-aliases", "real-restore-gates"]))


if __name__ == "__main__":
    main()
