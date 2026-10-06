#!/usr/bin/env python3
"""Freeze the accepted matrix into an isolated, explicit three-tier copy plan.

This command writes local configuration only; it does not contact MinIO.
"""
import argparse
import copy
import csv
import json
import pathlib
import re


OFFSETS = {"full-128k": 1, "full-4k": 2, "full-dedup-4k": 3,
           "no-image-4k": 4, "partial-4k": 5, "full-dedup-oracle": 6}


def plan_corpus(workloads, matrix, corpora, backend, port_bases, data_root, run_id):
    if not re.fullmatch(r"[a-z0-9][a-z0-9-]{0,60}", run_id):
        raise ValueError("invalid run id")
    if len(workloads["workloads"]) != 17 or len(matrix["systems"]) != 6:
        raise ValueError("expected frozen 17-workload/six-system input")
    if set(port_bases) != {512, 2048, 3072}:
        raise ValueError("all three port bases must be explicit")
    if sorted(w["vm_mib"] for w in workloads["workloads"]) != [512]*13+[2048]*3+[3072]:
        raise ValueError("wrong tier distribution")
    if any(w["request_path"] != "direct" for w in workloads["workloads"]):
        raise ValueError("not the accepted direct-path inputs")
    by_id = {row["corpus_id"]: row for row in corpora}
    if len(by_id) != 18:
        raise ValueError("expected six stores per tier, including the canonical oracle source")
    old_ports = {int(row["port"]) for row in corpora}
    ports = [base+i for base in port_bases.values() for i in OFFSETS.values()]
    if len(set(ports)) != 18 or any(p in old_ports or p < 1024 or p > 65535 for p in ports):
        raise ValueError("new port plan overlaps itself or the old corpus")
    result_workloads, result_matrix = copy.deepcopy(workloads), copy.deepcopy(matrix)
    result_matrix["ws_layout"] = "streams8-v1"
    for s in result_matrix["systems"]:
        if s["id"] == "full-dedup-zstd3" and (s["security"] != "partial" or not s["ws_coalescing"]):
            raise ValueError("Full Dedup must remain the partial-policy transfer-view oracle")
        s["ws_layout"] = "streams8-v1" if s["ws_coalescing"] else "not-applicable"
        s["ws_streams"] = 8 if s["ws_coalescing"] else 0
        s["paper_name"] = {"ws-zstd3": "Sabre", "no-image-zstd3": "SplitSnap-"}.get(s["id"], s["paper_name"])
        s["corpus_id"] += "-streams8-"+run_id
    jobs, new_corpora = [], []
    for tier, expected in ((512, 13), (2048, 3), (3072, 1)):
        ws = [w for w in workloads["workloads"] if w["vm_mib"] == tier]
        assert len(ws) == expected
        for kind, offset in OFFSETS.items():
            old = by_id[f"{kind}-tier{tier}"]
            corpus_id = old["corpus_id"]+"-streams8-"+run_id
            port = port_bases[tier]+offset
            container = f"snapshare-streams8-{run_id}-{kind}-{tier}"
            data_dir = str(pathlib.PurePosixPath(data_root)/corpus_id)
            new = dict(old, corpus_id=corpus_id, port=port, container=container, data_dir=data_dir)
            new_corpora.append(new)
            prefixes = ["_chunks_zstd_v1_l3/", "base/"]
            # Only the canonical store needs a raw page namespace for oracle validation.
            # Runtime native pages are already represented by their unchanged compressed blobs.
            if kind == "full-dedup-4k":
                prefixes.append("_chunks/")
            if kind in ("no-image-4k", "partial-4k", "full-dedup-oracle"):
                prefixes.append("ws_shared/base_rootfs/")
                if kind != "no-image-4k":
                    prefixes += ["ws_shared/images/"+name+"/" for name in sorted({w["image_inventory"] for w in ws})]
            prefixes += [w["snapshot"]+"/" for w in ws]
            mode = {"full-4k": "full", "no-image-4k": "private", "partial-4k": "private",
                    "full-dedup-oracle": "view"}.get(kind, "none")
            jobs.append(dict(corpus_id=corpus_id, tier=tier, kind=kind,
                             source=f"{backend}:{old['port']}", destination=f"{backend}:{port}",
                             container=container, data_dir=data_dir, prefixes=prefixes,
                             transcode_mode=mode, workloads={"workloads": ws}))
            for w in result_workloads["workloads"]:
                if w["vm_mib"] != tier:
                    continue
                for override in w["corpus_overrides"].values():
                    if override["corpus_id"] == old["corpus_id"]:
                        override.update(corpus_id=corpus_id, port=port, container=container)
    rows = []
    for w in result_workloads["workloads"]:
        for s in result_matrix["systems"]:
            override = w["corpus_overrides"][s["id"]]
            rows.append(dict(profile=w["profile"], snapshot=w["snapshot"], tier=w["vm_mib"],
                             system=s["id"], security=s["security"], chunk_size=s["chunk_size"],
                             ws_layout=s["ws_layout"], ws_streams=s["ws_streams"], **override))
    assert len(rows) == 102
    return dict(version=1, run_id=run_id, ws_layout="streams8-v1", backend=backend,
                tier_port_bases=port_bases, jobs=jobs, configuration_rows=rows,
                workloads=result_workloads, matrix=result_matrix, corpora=new_corpora)


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("--source-root", required=True, type=pathlib.Path)
    p.add_argument("--output", required=True, type=pathlib.Path)
    p.add_argument("--run-id", required=True)
    p.add_argument("--backend", required=True)
    p.add_argument("--port-bases", required=True, help='JSON e.g. {"512":9560,"2048":9570,"3072":9580}')
    p.add_argument("--data-root", required=True)
    a = p.parse_args()
    root = a.source_root
    with (root/"corpora.csv").open() as f:
        corpora = list(csv.DictReader(f))
    result = plan_corpus(json.loads((root/"workloads.json").read_text()),
                         json.loads((root/"matrix.json").read_text()), corpora, a.backend,
                         {int(k): int(v) for k, v in json.loads(a.port_bases).items()}, a.data_root, a.run_id)
    a.output.mkdir(parents=True, exist_ok=False)
    for name, value in (("plan.json", result), ("workloads.json", result["workloads"]),
                        ("matrix.json", result["matrix"]),
                        ("direct_requests.json", json.loads((root/"direct_requests.json").read_text()))):
        with (a.output/name).open("x") as f:
            json.dump(value, f, indent=2)
            f.write("\n")
    with (a.output/"corpora.csv").open("x") as f:
        writer = csv.DictWriter(f, fieldnames=list(corpora[0]))
        writer.writeheader()
        writer.writerows(result["corpora"])
    for job in result["jobs"]:
        with (a.output/(job["corpus_id"]+".workloads.json")).open("x") as f:
            json.dump(job["workloads"], f, indent=2)
            f.write("\n")
    print(f"PLAN_ONLY jobs={len(result['jobs'])} configurations={len(result['configuration_rows'])} output={a.output}")


if __name__ == "__main__":
    main()
