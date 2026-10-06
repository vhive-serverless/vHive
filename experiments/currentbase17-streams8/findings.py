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
    lines = ["# 八长流正式结果：比较与限制", "",
             "来源为本包 measured-medians.csv、figure9-10-footprint.csv 和 figure13-payload.csv；",
             "生成前由正式聚合器验收全部103点。每点slots30–59用于统计，全部60次记录保留。",
             "本报告不独立证明实验完成，仍需检查来源、原始失败记录及逐项发布验收。", "",
             "## 同轮六系统 E2E", "",
             "以下是17个workload各自E2E median的算术平均，不是所有调用混在一起的median。",
             "正的减少率表示SplitSnap更快，负值表示更慢；不截成0，也不筛除退化workload。", "",
             "| 系统 | E2E raw mean (ms) | SplitSnap相对该系统减少 (%) |",
             "| --- | ---: | ---: |"]
    for system, label in zip(ids, labels):
        lines.append(f"| {label} | {means[system]:.6f} | {reduction(means[system], means[current]):.3f} |")
    lines += ["", "套件减少率 = 1 - mean(SplitSnap E2E)/mean(baseline E2E)。",
              "最大改善率与逐workload改善率平均不是这个数，下面分别保留原始分布。", "",
              "| 比较对象 | 更快 / 相等 / 更慢 workload数 | 最小 / 最大减少率 (%) |",
              "| --- | --- | --- |"]
    for system, label in (("chunks-128k-zstd3", "Chunks"), ("ws-zstd3", "Sabre")):
        deltas = [reduction(times[system, p], times[current, p]) for p in profiles]
        counts = [sum(d > 0 for d in deltas), sum(d == 0 for d in deltas), sum(d < 0 for d in deltas)]
        lines.append(f"| {label} | {counts[0]} / {counts[1]} / {counts[2]} | {min(deltas):.3f} / {max(deltas):.3f} |")
    lines += ["", "| Workload | Chunks ms | Sabre ms | SplitSnap ms | 相对Chunks减少 (%) | 相对Sabre减少 (%) |",
              "| --- | ---: | ---: | ---: | ---: | ---: |"]
    for p in profiles:
        c, w, s = [times[system, p] for system in ("chunks-128k-zstd3", "ws-zstd3", current)]
        lines.append(f"| {p} | {c:.6f} | {w:.6f} | {s:.6f} | {reduction(c, s):.3f} | {reduction(w, s):.3f} |")

    footprint = {row["system"]: row for row in reports["footprint"]["systems"]}
    payload = {(row["system"], row["profile"]): row["compressed_payload_bytes"] for row in reports["payload"]["rows"]}
    payload_means = {s: statistics.mean(payload[s, p] for p in profiles) for s in ids}
    lines += ["", "## 相对Sabre的容量和传输量", "",
              "| 指标 | SplitSnap减少 (%) | 来源与口径 |", "| --- | ---: | --- |",
              f"| Remote storage | {reduction(footprint['WS']['total_storage_bytes'], footprint['SplitSnap']['total_storage_bytes']):.3f} | Figure9，snapshot + coalesced WS内容容量 |",
              f"| Local cache | {reduction(footprint['WS']['active_cache_bytes'], footprint['SplitSnap']['active_cache_bytes']):.3f} | Figure10，active WS容量模型，不是RSS |",
              f"| Mean fetch payload | {reduction(payload_means['ws-zstd3'], payload_means[current]):.3f} | Figure13，先平均压缩payload字节，再算比例；不含recipe/index/manifest |",
              "", "## 不作出的结论与失败边界", "",
              "- 这是同轮系统/policy比较，不是旧小帧与新八流的matched布局A/B。候选还含缓存/生命周期修正，不能将与旧图的全部差异归因于八流。",
              "- 保留所有退化和长尾。正式聚合不接受失败/不完整点；旧INVALID与诊断窗口另存，不能因本包成功而转正，也不能混入这套统计。",
              "- WS content与嵌套decode wall包含等网络和SHA，不能相减成纯网络或无SHA解压。Figure11蓝段还包含metadata，Execution为组件残差。",
              "- Full Dedup为预生成transfer-view oracle，排除离线assembly；其payload可与SplitSnap完全相同。容量模型与实际RSS、trace驻留、全部网络字节不同。",
              "- 主矩阵absolute1RPS可能有调用/清理重叠；没有据此生成隔离BW或C3/r12 trace结论。",
              "- 专属service CPU/peak覆盖启动到收尾，不是每次decoder的CPU/RSS；VM forced termination不能改称graceful。",
              "", "完整计时与公式见 [METHODS.md](METHODS.md)。", ""]
    return "\n".join(lines)
