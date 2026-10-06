#!/usr/bin/env python3
"""Audit real relay/MinIO evidence; a process exit alone is not a valid point."""
import argparse
import csv
import json
import pathlib
import re
import math

from runtime_probe import idle


def messages(log):
    out = []
    decoder = json.JSONDecoder()
    for line in log.splitlines():
        pos = line.find('msg="')
        if pos >= 0:
            message, _ = decoder.raw_decode(line[pos + 4:])
            out.append((line, message))
        else:
            out.append((line, line))
    return out


def stream_pairs(rows, expected, prefix):
    """Join by snapshot revision: concurrent calls can interleave the two markers."""
    decodes, samples = {}, {}
    for _, msg in rows:
        if not msg.startswith(("ZSTD_WS_DECODE ", "ZSTD_WS_STREAM_STATS ")):
            continue
        header = msg.split(" stats=", 1)[0]
        fields = dict(re.findall(r"(\w+)=([^ ]+)", header))
        revision = fields.get("revision", "")
        if not revision.startswith(prefix) or not revision:
            raise ValueError("unexpected WS revision: " + revision)
        if fields.get("layout") != "streams8-v1":
            raise ValueError("unexpected WS layout")
        target = decodes if msg.startswith("ZSTD_WS_DECODE ") else samples
        if revision in target:
            raise ValueError("duplicate WS marker for " + revision)
        target[revision] = fields if target is decodes else json.loads(msg.split(" stats=", 1)[1])
    if len(decodes) != expected or decodes.keys() != samples.keys():
        raise ValueError("WS marker revision sets/count differ")
    return [(revision, fields, samples[revision]) for revision, fields in decodes.items()]


def formal_plan(rows, inventory, aliases, config, system, profile):
    """Bind the window to frozen objects and slots, not merely its own log markers."""
    if (inventory["layout"] != "streams8-v1" or len(inventory["rows"]) != 102 or
            aliases["layout"] != inventory["layout"] or aliases["run_id"] != inventory["run_id"] or
            config["run_id"] != inventory["run_id"] or aliases["materialized"] is not True):
        raise ValueError("complete matching inventory/materialized aliases required")
    selected = [r for r in inventory["rows"] if (r["system"], r["profile"]) == (system, profile)]
    if len(selected) != 1:
        raise ValueError("matrix point is not unique")
    point = selected[0]
    for setting, key in (("minio_endpoint", "endpoint"), ("security", "security"),
                         ("vm_mib", "tier"), ("chunk_size", "chunk_size")):
        if config[setting] != point[key]:
            raise ValueError("point configuration differs: " + setting)
    if config["ws_coalescing"] != (point["ws_layout"] == "streams8-v1"):
        raise ValueError("point WS mode differs")
    expected = {}
    source_entries = {e["key"]: e for e in inventory["alias_source_entries"][point["corpus_id"]].values()
                      if e["key"].startswith(point["snapshot"] + "/")}
    for a in aliases["aliases"]:
        if (a["endpoint"], a["source_revision"]) != (point["endpoint"], point["snapshot"]):
            continue
        slot = a["slot"]
        name = f"{point['snapshot']}-{aliases['tag']}-{slot}"
        if (slot in expected or a["alias_revision"] != name or a["corpus_id"] != point["corpus_id"] or
                {e["key"]: e for e in a["source_entries"]} != source_entries):
            raise ValueError("alias identity/source entries differ")
        expected[slot] = name
    if set(expected) != set(range(60)) or len(rows) != 60:
        raise ValueError("exact 60 aliases/invocations required")
    seen = set()
    for r in rows:
        slot, index = int(r["slot"]), int(r["index"])
        suffix = r["revision"].rsplit("-", 2)
        if (slot in seen or slot not in expected or index != slot + 1 or len(suffix) != 3 or
                suffix[0] != expected[slot] or not suffix[1].isdigit() or suffix[2] != f"{index:05d}"):
            raise ValueError("invocation slot/alias/suffix differs from plan")
        seen.add(slot)
    return point, sorted(expected.values())


def vm_lifecycles(log, expected_revisions, coalesced):
    """Require complete per-VM proof, including teardown after the HTTP reply."""
    created, components, invoked, stopped, faults = {}, {}, {}, set(), set()
    terminated = {}
    for line, msg in messages(log):
        m = re.fullmatch(r"created VM with ID (\S+) and IP \S+ for revision (\S+)", msg)
        if m:
            vm, revision = m.groups()
            if vm in created:
                raise ValueError("duplicate VM identity")
            created[vm] = revision.rsplit("-", 2)[0]
        m = re.fullmatch(r"RESTORE_COMPONENTS revision=(\S+) vm_id=(\S+) metrics_us=(.+)", msg)
        if m:
            revision, vm, encoded = m.groups()
            if vm in components:
                raise ValueError("duplicate VM components")
            values = json.loads(encoded)
            required = {"LoadVMM", "GetWorkingSetPages", "GetUffdMemoryContent"}
            if coalesced:
                required.add("GetWorkingSetContent")
            if not required <= values.keys() or any(not math.isfinite(v) or v < 0 for v in values.values()):
                raise ValueError("invalid/missing restore component")
            components[vm] = (revision, values)
        m = re.fullmatch(r'Invocation to (\S+) completed in .+ with HTTP status 200 and gRPC status "(0|)"', msg)
        if m:
            if m[1] in invoked:
                raise ValueError("duplicate VM invocation")
            invoked[m[1]] = msg
        m = re.fullmatch(r"VM_TERMINATION_CONFIRMED vm_id=(\S+) method=(graceful|forced) processes_absent=true uffd_released=true", msg)
        if m:
            if m[1] in terminated:
                raise ValueError("duplicate VM termination proof")
            terminated[m[1]] = m[2]
        if msg == "Stopped VM successfully":
            m = re.search(r'vmID=([^\s"]+)', line)
            if m:
                stopped.add(m[1])
        if re.match(r"Handled [0-9]+ page faults", msg):
            m = re.search(r"uffd=/tmp/(.*?)\.uffd\.sock", line)
            if m:
                faults.add(m[1])
        if "level=error" in line and ("failed to stop firecracker-containerd VM" in msg or "forcefully terminated VM" in msg):
            raise ValueError("failed VM teardown is not a formal-point exception")
    if (sorted(created.values()) != sorted(expected_revisions) or
            set(created) != set(components) or set(created) != set(invoked) or
            set(created) != stopped or set(created) != faults or set(created) != set(terminated)):
        raise ValueError("per-VM restore/invocation/fault/teardown identities differ")
    for vm, revision in created.items():
        if components[vm][0] != revision:
            raise ValueError("component revision differs from VM revision")
    return dict(vms=len(created), successful_stops=len(stopped), released_uffd_records=len(faults),
                forced_terminations=sum(method == "forced" for method in terminated.values()),
                identity="request revision -> VM -> components/invocation/StopVM/UFFD")


def audit(log, stats, calls, coalesced, prefix, mode="remote", manifest=None):
    rows = messages(log)
    counts = {}
    patterns = dict(downloaded=r"Downloaded snapshot for rev " + re.escape(prefix),
                    loaded=r"Loaded snapshot for rev " + re.escape(prefix),
                    created=r"created VM with ID .* for revision " + re.escape(prefix),
                    ws_pages=r"^GetWorkingSetPages:", uffd=r"^(UffdPrepareDelay|GetUffdMemoryContent):",
                    load_vmm=r"^LoadVMM:", inserted=r"Pre-inserting working set",
                    faults=r"Handled [0-9]+ page faults", invoked=r"Invocation to .* completed in")
    if mode == "local":
        patterns.pop("downloaded")
    for name, pattern in patterns.items():
        counts[name] = sum(bool(re.search(pattern, msg)) for _, msg in rows)
        if counts[name] != calls:
            raise ValueError(f"{name}: {counts[name]} != {calls}")
    remote = sum("Using remote snapshot for rev " + prefix in msg for _, msg in rows)
    local = sum("Using snapshot for rev " + prefix in msg for _, msg in rows)
    if (remote, local) != ((calls, 0) if mode == "remote" else (0, calls)):
        raise ValueError(f"wrong local/remote path: {local}/{remote}")
    stopped = set()
    for line, msg in rows:
        if "Orchestrator received StopVM" in msg:
            match = re.search(r'vmID=([^ "\s]+)', line)
            if match:
                stopped.add(match[1])
        if "Relay args:" in msg or re.search(r"level=(panic|fatal)|Snapshot (Download|Load) Error", line):
            raise ValueError("helper/fatal marker: " + line)
        if "level=error" in line:
            if "context canceled" in line:
                raise ValueError("request cancellation leaked into runtime lifecycle: " + line)
            # Same narrow post-teardown UFFD exceptions as the accepted runner.
            if "failed to stop firecracker-containerd VM" in msg or "forcefully terminated VM" in msg:
                continue
            if "UFFD copy failed: no such process" in msg or "UFFD copy returned non-positive value: 0" in msg:
                match = re.search(r"uffd=/tmp/(.*?)\.uffd\.sock", line)
                if match and match[1] in stopped:
                    continue
            raise ValueError("unexpected relay error: " + line)
    expected = calls if coalesced else 0
    pairs = stream_pairs(rows, expected, prefix)
    payload_bytes = 0
    early_output_streams = 0
    for revision, fields, sample in pairs:
        if fields.get("layout") != "streams8-v1" or fields.get("streams") != "8" or fields.get("fetchers") != "8" or fields.get("source") != mode:
            raise ValueError("not the required eight-stream path: " + revision)
        if len(sample["streams"]) != 8 or {s["index"] for s in sample["streams"]} != set(range(8)):
            raise ValueError("stream index/count mismatch")
        if any(not s["success"] or s["range_opens"] != 1 or s["read_bytes"] <= 0 for s in sample["streams"]):
            raise ValueError("not eight complete long range reads")
        total = sum(s["read_bytes"] for s in sample["streams"])
        if total != int(fields["compressed_bytes"]):
            raise ValueError("per-stream byte sum mismatch")
        if manifest is not None:
            measured = {s["index"]: s["read_bytes"] for s in sample["streams"]}
            frozen = {s["index"]: s["compressed_size"] for s in manifest["streams"]}
            if (measured != frozen or int(fields["raw_bytes"]) != manifest["raw_size"] or
                    total != manifest["compressed_size"]):
                raise ValueError("stream bytes differ from frozen manifest: " + revision)
        payload_bytes += total
        early_output_streams += sum(0 < s["first_output_us"] < s["read_done_us"] for s in sample["streams"])
    for field in ("requests", "bytes"):
        if sum(c[field] for c in stats["classes"].values()) != stats["total"][field]:
            raise ValueError("counter class sum mismatch")
    if mode == "local":
        if stats.get("storage_disabled") is not True:
            raise ValueError("strict all-local must have no remote object store")
        if stats["total"] != {"requests": 0, "bytes": 0}:
            raise ValueError("all-local performed remote reads")
    else:
        if stats["total"]["requests"] <= 0 or stats["total"]["bytes"] <= 0:
            raise ValueError("remote point read no bytes")
        if coalesced and (stats["classes"]["working_set_payload"]["bytes"] != payload_bytes or
                          stats["classes"]["working_set_payload"]["requests"] != calls * 8):
            raise ValueError("remote payload accounting differs from eight streams")
    return dict(calls=calls, mode=mode, component_counts=counts, layout="streams8-v1" if coalesced else "not-applicable",
                decoded_revisions=sorted(revision for revision, _, _ in pairs),
                decoded_payload_bytes=payload_bytes, streams_with_output_before_final_read=early_output_streams,
                counter_definition="successful application reads, not all HTTP attempts")


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("root", type=pathlib.Path)
    p.add_argument("--prefix", required=True)
    p.add_argument("--calls", type=int, default=60)
    p.add_argument("--coalesced", action="store_true")
    p.add_argument("--mode", choices=("remote", "local"), default="remote")
    p.add_argument("--functional", action="store_true")
    p.add_argument("--inventory", type=pathlib.Path)
    p.add_argument("--aliases", type=pathlib.Path)
    p.add_argument("--point-config", type=pathlib.Path)
    p.add_argument("--system")
    p.add_argument("--profile")
    a = p.parse_args()
    manifest, expected_revisions = None, None
    if a.functional:
        if a.calls != 1 or not json.loads((a.root / "invocation-result.json").read_text())["success"]:
            raise ValueError("one successful functional invocation required")
    else:
        with (a.root / "invocations.tsv").open() as f:
            rows = list(csv.DictReader(f, delimiter="\t"))
        if a.calls != 60 or len(rows) != 60 or len({r["revision"] for r in rows}) != 60:
            raise ValueError("formal point requires 60 distinct revisions")
        if any(r["exit_status"] != "0" or r["reply_ok"] != "1" or r["direct_native"] != "1" for r in rows):
            raise ValueError("unsuccessful/non-native invocation")
        if not all((a.inventory, a.aliases, a.point_config, a.system, a.profile)):
            raise ValueError("formal validation requires inventory, aliases, point-config, system and profile")
        config = json.loads(a.point_config.read_text())
        point, expected_revisions = formal_plan(rows, json.loads(a.inventory.read_text()),
                                               json.loads(a.aliases.read_text()), config, a.system, a.profile)
        if a.mode != config["mode"] or a.coalesced != config["ws_coalescing"]:
            raise ValueError("validator/runtime mode differs")
        if any(not revision.startswith(a.prefix) for revision in expected_revisions):
            raise ValueError("validator prefix does not cover planned aliases")
        manifest = point.get("manifest")
    result = audit((a.root / "relay.log").read_text(),
                   json.loads((a.root / "remote-fetch-stats.json").read_text()),
                   a.calls, a.coalesced, a.prefix, a.mode, manifest)
    if not a.functional and a.coalesced:
        if result["decoded_revisions"] != expected_revisions:
            raise ValueError("decoded snapshots differ from invocation aliases")
    result["purpose"] = "functional only" if a.functional else "30 warmup + 30 measured"
    if not a.functional:
        if not idle(json.loads((a.root / "runtime-state-final.json").read_text())):
            raise ValueError("formal window has unfinished runtime/UFFD/cache work")
        result["lifecycle"] = vm_lifecycles((a.root / "relay.log").read_text(), expected_revisions, a.coalesced)
        result.update(system=a.system, profile=a.profile, inventory=str(a.inventory), aliases=str(a.aliases),
                      warmup_slots=list(range(30)), measured_slots=list(range(30, 60)),
                      sample_order="frozen invocation slot; never completion order")
    with (a.root / "validation.json").open("x") as f:
        json.dump(result, f, indent=2)
        f.write("\n")
    print(json.dumps(result))


if __name__ == "__main__":
    main()
