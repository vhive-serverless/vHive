"""Freeze the actual static reports with the measured run, not a mutable path."""
import json


def load_accounting(provenance):
    return {name: json.loads((provenance / (name + ".json")).read_text())
            for name in ("footprint", "payload")}


def validate_accounting(reports, inventory):
    footprint, payload = reports["footprint"], reports["payload"]
    if (footprint["layout"] != inventory["layout"] or payload["layout"] != inventory["layout"] or
            payload["run_id"] != inventory["run_id"]):
        raise ValueError("accounting layout/run differs from inventory")
    expected = {(r["system"], r["profile"]): r for r in inventory["rows"]}
    actual = {(r["system"], r["profile"]): r for r in payload["rows"]}
    if len(actual) != len(payload["rows"]) or actual.keys() != expected.keys():
        raise ValueError("accounting matrix differs from inventory")
    profiles = {r["profile"] for r in inventory["rows"]}
    if len(footprint["profiles"]) != len(profiles) or set(footprint["profiles"]) != profiles:
        raise ValueError("accounting profiles differ from inventory")
    for key, row in actual.items():
        source = expected[key]
        if (row["snapshot"], row["endpoint"], row["layout"]) != (
                source["snapshot"], source["endpoint"], source["ws_layout"]):
            raise ValueError("accounting source identity differs: " + repr(key))
        keys = row["payload_keys"]
        if len(set(keys)) != len(keys) or len(keys) != row["unique_payload_objects"]:
            raise ValueError("accounting object count differs: " + repr(key))
        size = sum(payload["object_sizes"][row["endpoint"] + "/" + name] for name in keys)
        if size != row["compressed_payload_bytes"]:
            raise ValueError("accounting payload sizes do not reconcile: " + repr(key))
        if source["ws_layout"] == "streams8-v1" and (
                keys != [source["payload_key"]] or size != source["compressed_bytes"]):
            raise ValueError("accounting coalesced payload differs: " + repr(key))


def frozen_accounting(root):
    frozen = json.loads((root / "frozen.json").read_text())
    reports = frozen["accounting"]
    validate_accounting(reports, frozen["inventory"])
    return reports
