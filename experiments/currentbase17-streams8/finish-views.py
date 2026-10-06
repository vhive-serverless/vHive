#!/usr/bin/env python3
"""Validate the formal Full Dedup views after all corpus copies/transcodes finish."""
import argparse
import fcntl
import json
import pathlib

from prepare import execute, publish


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("run_root", type=pathlib.Path)
    a = p.parse_args()
    root = a.run_root
    if not root.is_absolute() or root.parent != pathlib.Path("/users/Liquidz/streams8"):
        raise ValueError("explicit streams8 root required")
    ready = json.loads((root / "COPIES_TRANSCODES_COMPLETE.json").read_text())
    plan_path = root / "config/plan.json"
    plan = json.loads(plan_path.read_text())
    if ready["run_id"] != plan["run_id"] or ready["copies"] != 18 or ready["transcode_jobs"] != 9:
        raise ValueError("preparation incomplete")
    lock = (root / "finish-views.lock").open("a")
    fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
    execute(root, "view-copy", [str(root / "bin/copy-view-streams8"), "-plan", str(plan_path),
                               "-report", str(root / "stages/view-copy.report.json")])
    reports = []
    for tier in (512, 2048, 3072):
        canonical = next(j for j in plan["jobs"] if j["tier"] == tier and j["kind"] == "full-dedup-4k")
        view = next(j for j in plan["jobs"] if j["tier"] == tier and j["kind"] == "full-dedup-oracle")
        report = root / "stages" / f"oracle-tier{tier}.report.json"
        execute(root, f"oracle-tier{tier}", [str(root / "bin/materialize-full-dedup-streams8"),
                "-minioURL", canonical["destination"], "-referenceMinioURL", view["destination"],
                "-batchWorkloads", str(root / "config" / (view["corpus_id"] + ".workloads.json")),
                "-verifyCanonical", "working-set", "-workers", "28", "-zstdLevel", "3",
                "-zstdWSLayout", "streams8-v1", "-report", str(report)])
        reports.append(str(report))
    publish(root / "ORACLE_VIEWS_COMPLETE.json", dict(run_id=plan["run_id"], layout="streams8-v1",
            reports=reports, canonical_verification="all recipe keys present; WS content verified; immutable non-WS copies retain source ETag proof",
            pending=["actual-102-row-inventory", "aliases", "runtime-and-formal-gates"]))


if __name__ == "__main__":
    main()
