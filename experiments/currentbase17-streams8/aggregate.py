"""New Figures9-13 from eight-stream evidence; legacy inputs/figures are read-only.

Static-only emits Figures9/10/13 after complete actual-store accounting. Full mode
also requires and re-audits all 103 formal points before emitting Figures11/12.
"""
import argparse
import csv
import json
import pathlib
import shutil
import statistics

import matplotlib
matplotlib.use("Agg")
import matplotlib.pyplot as plt
import numpy as np

from metrics import medians, parse_metrics, quality_summary
from run_matrix import Matrix, ordered_points
from accounting_inputs import load_accounting, frozen_accounting
from findings import build_findings
from platform_evidence import validate_point_platform

HERE = pathlib.Path(__file__).resolve().parent
PROVENANCE = HERE / "provenance/20260910-r1"
SYSTEMS = ["chunks-128k-zstd3", "pages-4k-zstd3", "ws-zstd3", "no-image-zstd3", "splitsnap-zstd3", "full-dedup-zstd3"]
LABELS = dict(zip(SYSTEMS, ["Chunks", "Pages", "Sabre", "SplitSnap-", "SplitSnap", "Full Dedup"]))
COLORS = dict(zip(SYSTEMS, ["C0", "C1", "C2", "C5", "C4", "C6"]))
SHORT = ["Image-Go", "Image-Py", "Video Proc.", "VideoAn", "AES-Go", "AES-Py", "AES-NJS",
         "Auth-Go", "Auth-NJS", "Auth-Py", "Fib-Go", "Fib-NJS", "Fib-Py", "Currency", "Email", "Prod. Cat.", "Shipping"]


def write_csv(path, rows):
    with path.open("x", newline="") as f:
        writer = csv.DictWriter(f, fieldnames=list(rows[0]))
        writer.writeheader()
        writer.writerows(rows)


def normalized_rows(values, systems, profiles, denominator):
    rows = []
    for profile in profiles + ["Mean"]:
        scalars = {s: statistics.mean(values[s, p] for p in profiles) if profile == "Mean" else values[s, profile]
                   for s in SYSTEMS}
        for s in systems:
            rows.append(dict(profile=profile, system=s, raw_value=scalars[s],
                             denominator=scalars[denominator], normalized=scalars[s]/scalars[denominator]))
    return rows


def save(fig, out, name):
    for suffix in ("png", "pdf"):
        fig.savefig(out / "figures" / f"{name}.{suffix}", dpi=220, bbox_inches="tight", pad_inches=.06)
    plt.close(fig)


def grouped(rows, systems, profiles, ylabel, out, name):
    fig, ax = plt.subplots(figsize=(14, 4.1))
    x, width = np.arange(len(profiles)+1), .15
    for i, s in enumerate(systems):
        ys = [next(r["normalized"] for r in rows if r["system"] == s and r["profile"] == p) for p in profiles+["Mean"]]
        ax.bar(x+(i-(len(systems)-1)/2)*width, ys, width=width, color=COLORS[s], label=LABELS[s])
    ax.set_xticks(x, SHORT+["Mean"], rotation=60, ha="right")
    ax.set_ylabel(ylabel)
    ax.set_ylim(bottom=0)
    ax.margins(x=.005)
    ax.grid(axis="y", alpha=.45)
    ax.set_axisbelow(True)
    ax.legend(ncol=len(systems), loc="lower center", bbox_to_anchor=(.5, 1.01), frameon=True,
              columnspacing=1.2, handlelength=1.4)
    save(fig, out, name)


def static_figures(out, profiles, reports):
    footprint, payload = reports["footprint"], reports["payload"]
    if footprint["layout"] != "streams8-v1" or payload["layout"] != "streams8-v1" or payload["run_id"] != "20260910-r1":
        raise ValueError("not the frozen eight-stream accounting")
    if set(footprint["profiles"]) != set(profiles):
        raise ValueError("footprint workload set differs")
    rows = footprint["systems"]
    if [r["system"] for r in rows] != ["Chunks", "Pages", "WS", "No-image", "SplitSnap", "Full Dedup"]:
        raise ValueError("footprint baseline mapping differs")
    write_csv(out/"figure9-10-footprint.csv", rows)
    x = np.arange(6)
    fig, ax = plt.subplots(figsize=(6.2, 3.6))
    bottom = np.array([r["snapshot_bytes"] for r in rows])/footprint["raw_full_snapshot_bytes"]
    extra = np.array([r["working_set_storage_bytes"] for r in rows])/footprint["raw_full_snapshot_bytes"]
    ax.bar(x, bottom, width=.63, label="Snapshots")
    ax.bar(x, extra, bottom=bottom, width=.63, label="Working sets", color="C1", hatch="/")
    ax.set_xticks(x, LABELS.values(), rotation=50, ha="right")
    ax.set_ylabel("Relative storage size")
    ax.legend(frameon=True, loc="lower center", bbox_to_anchor=(.5, 1.01), ncol=2, fontsize=13)
    ax.grid(axis="y", alpha=.45)
    ax.set_axisbelow(True)
    save(fig, out, "figure9_streams8_storage")
    fig, ax = plt.subplots(figsize=(6.2, 3.6))
    values = [r["cache_normalized_to_ws"] for r in rows]
    ax.bar(x, values, width=.63)
    ax.set_ylim(0, 1.5)
    ax.set_yticks([0, .5, 1, 1.5])
    ax.text(0, 1.48, f"↑{values[0]:.2f}×", ha="center", va="top")
    ax.set_xticks(x, LABELS.values(), rotation=50, ha="right")
    ax.set_ylabel("Relative cache\nrequirements")
    ax.grid(axis="y", alpha=.45)
    ax.set_axisbelow(True)
    save(fig, out, "figure10_streams8_cache")
    values = {(r["system"], r["profile"]): r["compressed_payload_bytes"] for r in payload["rows"]}
    if len(payload["rows"]) != 102 or set(values) != {(s,p) for s in SYSTEMS for p in profiles}:
        raise ValueError("actual payload matrix incomplete")
    fetch = normalized_rows(values, SYSTEMS[1:], profiles, SYSTEMS[0])
    write_csv(out/"figure13-payload.csv", fetch)
    grouped(fetch, SYSTEMS[1:], profiles, "Fetch size (norm. to Chunks)", out, "figure13_streams8_payload")


def formal_data(root, plan):
    frozen = json.loads((root/"frozen.json").read_text())
    if frozen["version"] not in ("gate8", "gate9", "gate10", "gate11", "gate12") or frozen["plan"] != plan:
        raise ValueError("formal version/plan differs")
    validator = Matrix.__new__(Matrix)
    validator.inventory, validator.aliases = frozen["inventory"], frozen["aliases"]
    summaries, scalar_rows, sample_rows, quality, resources = {}, [], [], [], []
    for s, p, mode in ordered_points(plan):
        point = root/"points"/(s+"--"+p+"--"+mode)
        completion = json.loads((point/"POINT_COMPLETE.json").read_text())
        if (completion["system"], completion["profile"], completion["mode"], completion["version"]) != (s,p,mode,frozen["version"]):
            raise ValueError("completion identity/version differs")
        config = json.loads((point/"config.json").read_text())
        platform_observations = validate_point_platform(point)
        if frozen["version"] in ("gate11", "gate12"):
            shutdown = json.loads((point/"shutdown.json").read_text())
            if (shutdown["unit"] != config["unit"] or shutdown["binary"] != config["relay_binary"] or
                    not shutdown["processes_absent"] or shutdown["remaining_processes"] or not shutdown["vm_ids"]):
                raise ValueError("owned shutdown identity/evidence differs")
        resource = json.loads((point/"relay-resources.json").read_text())
        if resource["unit"] != config["unit"] or not resource["message"].startswith(config["unit"]+": Consumed "):
            raise ValueError("resource accounting identity differs")
        resources.append(dict(system=s,profile=p,mode=mode,
                              platform_observations_verified=platform_observations, **resource))
        validator.validate(point, config, s, p, mode)
        with (point/"invocations.tsv").open() as f:
            calls = list(csv.DictReader(f, delimiter="\t"))
        samples = parse_metrics((point/"relay.log").read_text(), calls, config["ws_coalescing"], mode)
        if s == "splitsnap-zstd3" and mode == "remote":
            inventory = next(r for r in frozen["inventory"]["rows"] if (r["system"],r["profile"])==(s,p))
            quality.append(dict(profile=p, **quality_summary(samples, inventory)))
        summary = medians(samples)
        summaries[s,p,mode] = summary
        scalar_rows.append(dict(system=s, profile=p, mode=mode, **summary))
        sample_rows.extend(dict(system=s, profile=p, mode=mode, **row) for row in samples)
    return summaries, scalar_rows, sample_rows, quality, resources


def formal_figures(data, out, profiles):
    summaries, scalar_rows, sample_rows, quality, resources = data
    write_csv(out/"components-per-call.csv", sample_rows)
    write_csv(out/"measured-medians.csv", scalar_rows)
    write_csv(out/"figure15-ws-quality.csv", quality)
    write_csv(out/"relay-lifetime-resources.csv", resources)
    values = {(s,p):summaries[s,p,"remote"]["relay_e2e"] for s in SYSTEMS for p in profiles}
    selected = [s for s in SYSTEMS if s != "pages-4k-zstd3"]
    e2e = normalized_rows(values, selected, profiles, "pages-4k-zstd3")
    write_csv(out/"figure12-e2e.csv", e2e)
    grouped(e2e, selected, profiles, "E2E time (norm. to Pages)", out, "figure12_streams8_e2e")
    aes = "aes-go-45000-45450"
    reference = summaries["ws-zstd3",aes,"remote"]["insert"]
    components = []
    for s, mode in [(s,"remote") for s in SYSTEMS]+[("splitsnap-zstd3","local")]:
        v = summaries[s,aes,mode]
        extra = max(0, v["insert"]-reference) if s in SYSTEMS[:2] else 0
        blue = sum(v[k] for k in ("download", "get_uffd", "get_ws_pages", "get_ws_content"))+extra
        restoration = max(0, v["load_vmm"]-extra)
        faults = max(0, v["page_fault_handler"]-v["insert"])
        execution = max(0, v["relay_e2e"]-blue-restoration-faults)
        if abs(blue+restoration+faults+execution-v["relay_e2e"]) > 1e-6:
            raise ValueError("component sum differs from measured relay E2E")
        components.append(dict(system=s, mode=mode, label=LABELS[s] if mode=="remote" else "All-local",
                               fetch_decompression=blue, restoration=restoration, execution=execution,
                               page_faults=faults, relay_e2e=v["relay_e2e"], preinsert_reclassified=extra))
    write_csv(out/"figure11-components.csv", components)
    fig, ax = plt.subplots(figsize=(9, 3.7))
    x, bottom = np.arange(7), np.zeros(7)
    for key, label, color in [("fetch_decompression","Fetch + Decompression","C0"),
            ("restoration","Restoration","C2"),("execution","Execution","C3"),("page_faults","Page Faults","C4")]:
        ys = np.array([r[key] for r in components])
        ax.bar(x, ys, bottom=bottom, label=label, color=color, width=.65)
        bottom += ys
    for i, y in enumerate(bottom): ax.text(i, y, f"{y:.1f}", ha="center", va="bottom")
    ax.set_xticks(x, [r["label"] for r in components], rotation=35, ha="right")
    ax.set_ylabel("Cold-start components (ms)")
    ax.set_ylim(0,max(bottom)*1.15)
    ax.legend(ncol=2, frameon=True, loc="lower center", bbox_to_anchor=(.5,1.01))
    ax.grid(axis="y", alpha=.45)
    ax.set_axisbelow(True)
    save(fig, out, "figure11_streams8_aes_components")


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("--results", type=pathlib.Path)
    p.add_argument("--output", type=pathlib.Path, required=True)
    p.add_argument("--static-only", action="store_true")
    a = p.parse_args()
    if not a.static_only and a.results is None: p.error("--results required for formal figures")
    plan = json.loads((HERE/"config/20260910-r1/plan.json").read_text())
    profiles = [w["profile"] for w in plan["workloads"]["workloads"]]
    if len(profiles)!=17 or len(set(profiles))!=17: raise ValueError("17 frozen profiles required")
    # Reject incomplete formal evidence before creating any apparent result pack.
    data = None if a.static_only else formal_data(a.results, plan)
    # Formal figures consume the exact reports embedded before measurement.
    # Static-only remains an explicitly separate accounting preview.
    reports = load_accounting(PROVENANCE) if a.static_only else frozen_accounting(a.results)
    a.output.mkdir(parents=True, exist_ok=False)
    (a.output/"figures").mkdir()
    plt.rcParams.update({"font.size":15, "axes.labelsize":17, "pdf.fonttype":42})
    static_figures(a.output, profiles, reports)
    if not a.static_only:
        formal_figures(data, a.output, profiles)
        (a.output/"FINDINGS.md").write_text(build_findings(profiles, data[0], reports))
    (a.output/"README.md").write_text(
        "# Eight-stream results\n\n"+
        ("Static accounting only; no new latency result is claimed.\n" if a.static_only else
         "All 103 formal points re-audited; slots 30–59 measured. Saved before/after platform records are checked for all three nodes (618 node observations); these snapshots are not continuous monitoring.\n")+
        "\nFigure9/10: cross-tier first-occurrence union. Figure13: compressed content payload only; recipe/index/manifest excluded.\n"
        "Formal static inputs are embedded in the run's frozen.json; mutable provenance files are used only by --static-only previews.\n"
        "Figure12/13 Mean: average raw per-workload scalars before normalization. Figure11: matched median components, nested WS timer not added twice.\n"
        "Full Dedup: canonical storage/cache oracle; prebuilt partial-format transfer view for runtime.\n"
        "Figure15 CSV updates extra UFFD-event statistics, preserving tails and validating WS/private counts against immutable inputs; these are events, not unique remote pages. The whole-WS ratio excludes recipe; legacy_private_plus_recipe_pct is an explicitly named historical normalization only, unrelated to Figure13 payload accounting.\n"
        "Old results and BW/C3/r12 trace are not relabelled as this implementation.\n")
    shutil.copyfile(HERE/"METHODS.md", a.output/"METHODS.md")
    with (a.output/"README.md").open("a") as readme:
        readme.write("\nSee [METHODS.md](METHODS.md) for exact component formulas, actual-size versus modeled capacity, timer boundaries, and provenance requirements.\n")
        if not a.static_only:
            readme.write("\nSee [FINDINGS.md](FINDINGS.md) for same-run comparisons, per-workload regressions, and limits on attribution.\n")
    print("STATIC_FIGURES_COMPLETE" if a.static_only else "FIGURES9_13_COMPLETE", a.output)


if __name__ == "__main__":
    main()
