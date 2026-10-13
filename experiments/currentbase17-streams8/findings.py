"""Paper-facing comparisons, including regressions, from accepted formal inputs."""
import statistics


def reduction(reference, value):
    return 100 * (1 - value / reference)


def build_findings(profiles, summaries, reports):
    """Called only after aggregate.formal_data accepts the entire matrix.

    Per-workload values are measured medians. Suite reductions are ratios of
    raw means, never means of per-workload percentage reductions.
    """
    ids = ("chunks-128k-zstd3", "pages-4k-zstd3", "ws-zstd3", "no-image-zstd3",
           "splitsnap-zstd3", "full-dedup-zstd3")
    labels = ("Chunks", "Pages", "Sabre", "SplitSnap-", "SplitSnap", "Full Dedup")
    times = {(s, p): summaries[s, p, "remote"]["relay_e2e"] for s in ids for p in profiles}
    means = {s: statistics.mean(times[s, p] for p in profiles) for s in ids}
    current = "splitsnap-zstd3"
    lines = ["# Eight-stream results: comparisons and limitations", "",
             "Sources: measured-medians.csv, figure9-10-footprint.csv and figure13-payload.csv in this result package.",
             "The formal aggregator validates all 103 points before generating this report. Slots 30–59 are measured; all 60 calls are retained.",
             "This report alone does not establish completion; provenance, original failure records and release acceptance still require review.", "",
             "## E2E across six systems in the same run", "",
             "Values are arithmetic means of the 17 per-workload E2E medians, not a median of pooled calls.",
             "Positive reductions mean SplitSnap is faster; negative values mean slower. Regressions are neither clipped to zero nor excluded.", "",
             "| System | E2E raw mean (ms) | SplitSnap reduction relative to this system (%) |",
             "| --- | ---: | ---: |"]
    for system, label in zip(ids, labels):
        lines.append(f"| {label} | {means[system]:.6f} | {reduction(means[system], means[current]):.3f} |")
    lines += ["", "Suite reduction = 1 - mean(SplitSnap E2E)/mean(baseline E2E).",
              "This differs from the maximum improvement or the mean of per-workload reductions; the distribution is retained below.", "",
              "| Baseline | Faster / equal / slower workloads | Minimum / maximum reduction (%) |",
              "| --- | --- | --- |"]
    for system, label in (("chunks-128k-zstd3", "Chunks"), ("ws-zstd3", "Sabre")):
        deltas = [reduction(times[system, p], times[current, p]) for p in profiles]
        counts = [sum(d > 0 for d in deltas), sum(d == 0 for d in deltas), sum(d < 0 for d in deltas)]
        lines.append(f"| {label} | {counts[0]} / {counts[1]} / {counts[2]} | {min(deltas):.3f} / {max(deltas):.3f} |")
    lines += ["", "| Workload | Chunks ms | Sabre ms | SplitSnap ms | Reduction vs Chunks (%) | Reduction vs Sabre (%) |",
              "| --- | ---: | ---: | ---: | ---: | ---: |"]
    for p in profiles:
        c, w, s = [times[system, p] for system in ("chunks-128k-zstd3", "ws-zstd3", current)]
        lines.append(f"| {p} | {c:.6f} | {w:.6f} | {s:.6f} | {reduction(c, s):.3f} | {reduction(w, s):.3f} |")

    footprint = {row["system"]: row for row in reports["footprint"]["systems"]}
    payload = {(row["system"], row["profile"]): row["compressed_payload_bytes"] for row in reports["payload"]["rows"]}
    payload_means = {s: statistics.mean(payload[s, p] for p in profiles) for s in ids}
    lines += ["", "## Capacity and transfer volume relative to Sabre", "",
              "| Metric | SplitSnap reduction (%) | Source and definition |", "| --- | ---: | --- |",
              f"| Remote storage | {reduction(footprint['WS']['total_storage_bytes'], footprint['SplitSnap']['total_storage_bytes']):.3f} | Figure9: snapshot + coalesced WS content capacity |",
              f"| Local cache | {reduction(footprint['WS']['active_cache_bytes'], footprint['SplitSnap']['active_cache_bytes']):.3f} | Figure10: active WS capacity model, not RSS |",
              f"| Mean fetch payload | {reduction(payload_means['ws-zstd3'], payload_means[current]):.3f} | Figure13: ratio of mean compressed payload bytes; excludes recipe/index/manifest |",
              "", "## Limits on interpretation and failure handling", "",
              "- This compares systems/policies in the same run, not a matched A/B of old small frames versus eight streams. Cache/lifecycle fixes are also present; not all differences from old figures can be attributed to eight streams.",
              "- All regressions and tails are retained. Formal aggregation rejects failed/incomplete points; old INVALID and diagnostic windows remain separate and are not validated by this package or included in its statistics.",
              "- WS content and nested decode wall times include network waits and SHA; subtracting them does not yield pure network time or SHA-free decompression. Figure11's blue component also includes metadata; Execution is a component residual.",
              "- Full Dedup uses a prebuilt transfer-view oracle, excluding offline assembly; its payload may equal SplitSnap's. Capacity models differ from RSS, trace residency and total network traffic.",
              "- The main matrix uses absolute 1 RPS, so calls/cleanup may overlap; it does not establish isolated bandwidth or C3/r12 trace results.",
              "- Dedicated-service CPU/peak covers startup through shutdown, not per-decoder CPU/RSS; forced VM termination must not be described as graceful.", ""]
    return "\n".join(lines)
