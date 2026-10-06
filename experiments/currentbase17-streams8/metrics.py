"""Identity-keyed measured components; never associate interleaved anonymous lines."""
import json
import math
import re
import statistics

from validate_point import messages


def duration_ms(value):
    units = {"ns": 1e-6, "us": 1e-3, "µs": 1e-3, "ms": 1, "s": 1000, "m": 60000}
    parts = re.findall(r"([0-9.]+)(ns|us|µs|ms|s|m)", value)
    if not parts or "".join(a+b for a, b in parts) != value:
        raise ValueError("unsupported duration: " + value)
    return sum(float(amount)*units[unit] for amount, unit in parts)


def parse_metrics(log, invocations, coalesced, mode):
    aliases = {r["revision"].rsplit("-", 2)[0]: int(r["slot"]) for r in invocations}
    if len(aliases) != 60 or set(aliases.values()) != set(range(60)):
        raise ValueError("exact 60-slot invocation identities required")
    vms, by_vm, by_alias, component_revisions = {}, {}, {}, {}

    def put(target, identity, key, value):
        row = target.setdefault(identity, {})
        if key in row or not math.isfinite(value) or value < 0:
            raise ValueError("duplicate/invalid metric: " + identity + "/" + key)
        row[key] = value

    for line, msg in messages(log):
        m = re.fullmatch(r"created VM with ID (\S+) and IP \S+ for revision (\S+)", msg)
        if m:
            if m[1] in vms:
                raise ValueError("duplicate VM")
            vms[m[1]] = m[2].rsplit("-", 2)[0]
        m = re.fullmatch(r"RESTORE_COMPONENTS revision=(\S+) vm_id=(\S+) metrics_us=(.+)", msg)
        if m:
            if m[2] in component_revisions:
                raise ValueError("duplicate components")
            component_revisions[m[2]] = m[1]
            for source, key in (("LoadVMM", "load_vmm"), ("GetWorkingSetPages", "get_ws_pages"),
                                ("GetUffdMemoryContent", "get_uffd"), ("GetWorkingSetContent", "get_ws_content")):
                values = json.loads(m[3])
                if source in values:
                    put(by_vm, m[2], key, values[source]/1000)
        m = re.fullmatch(r"Downloaded snapshot for rev (\S+) in ([0-9]+)", msg)
        if m:
            put(by_alias, m[1], "download", int(m[2])/1000)
        m = re.search(r"^ZSTD_WS_DECODE revision=(\S+).* elapsed_us=([0-9]+)", msg)
        if m:
            put(by_alias, m[1], "ws_decode_nested", int(m[2])/1000)
        sock = re.search(r"uffd=/tmp/([^\s.]+)\.uffd\.sock", line)
        if sock:
            m = re.search(r"Pre-inserting working set .* in ([^, ]+)", msg)
            if m:
                put(by_vm, sock[1], "insert", duration_ms(m[1]))
            m = re.search(r"Pre-inserting working set of ([0-9]+) pages", msg)
            if m:
                put(by_vm, sock[1], "inserted_ws_pages", int(m[1]))
            m = re.search(r"private page count: ([0-9]+)", msg)
            if m:
                put(by_vm, sock[1], "private_ws_pages", int(m[1]))
            m = re.fullmatch(r"Handled ([0-9]+) page faults in (\S+)", msg)
            if m:
                put(by_vm, sock[1], "page_fault_handler", duration_ms(m[2]))
                put(by_vm, sock[1], "page_fault_count", int(m[1]))
        m = re.match(r"Invocation to (\S+) completed in (\S+) with HTTP status", msg)
        if m:
            put(by_vm, m[1], "relay_e2e", duration_ms(m[2]))
    if (set(vms.values()) != set(aliases) or len(vms) != 60 or
            component_revisions != vms or set(by_vm) != set(vms) or not set(by_alias) <= set(aliases)):
        raise ValueError("metric/VM/invocation identities differ")
    out = []
    for vm, alias in vms.items():
        row = {**by_vm[vm], **by_alias.get(alias, {})}
        if mode == "local":
            row["download"] = 0.0
        if not coalesced:
            row["get_ws_content"] = row["ws_decode_nested"] = 0.0
        required = {"download", "get_uffd", "get_ws_pages", "get_ws_content", "load_vmm",
                    "insert", "inserted_ws_pages", "page_fault_handler", "page_fault_count", "relay_e2e", "ws_decode_nested"}
        if not required <= row.keys():
            raise ValueError(f"missing metrics for {vm}: {required-row.keys()}")
        out.append(dict(slot=aliases[alias], vm_id=vm, revision=alias, **row))
    return sorted(out, key=lambda row: row["slot"])


def medians(samples):
    measured = [row for row in samples if 30 <= row["slot"] <= 59]
    if len(measured) != 30:
        raise ValueError("30 measured slots required")
    keys = set(measured[0]) - {"slot", "vm_id", "revision"}
    return {key: statistics.median(row[key] for row in measured) for key in sorted(keys)}


def quality_summary(samples, inventory):
    """Figure15 event proxy, not distinct missing pages or network transfers."""
    ws, private, recipe = (inventory[k] for k in ("working_set_pages", "index_pages", "recipe_bytes"))
    if ws <= 0 or private*4096 != inventory["raw_bytes"] or recipe != inventory["tier"]*2**20//4096*16:
        raise ValueError("quality inventory differs from frozen raw/index/recipe")
    if len(samples) != 60 or {r["slot"] for r in samples} != set(range(60)):
        raise ValueError("quality requires all 60 identities")
    if any((r["inserted_ws_pages"], r.get("private_ws_pages")) != (ws,private) or
           r["page_fault_count"] < 1 for r in samples):
        raise ValueError("runtime insertion/fault counts differ from frozen WS")
    extras = [r["page_fault_count"]-1 for r in samples if 30 <= r["slot"] <= 59]
    return dict(measured_calls=30, ws_pages=ws, private_ws_pages=private, recipe_bytes=recipe,
                median_extra_events=statistics.median(extras), min_extra_events=min(extras),
                max_extra_events=max(extras), calls_over_100_extra_events=sum(n>100 for n in extras),
                whole_ws_pct=statistics.median(extras)/ws*100,
                legacy_private_plus_recipe_pct=statistics.median(extras)*4096/(private*4096+recipe)*100)
