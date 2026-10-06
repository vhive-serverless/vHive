"""Prepare matched AES all-local identities from a verified, same-byte local cache.

Only immutable files are hardlinked; this is preparation, not a cache-size estimate.
No remote access, re-encoding, source removal, or existing-cache overwrite occurs.
"""
import argparse
import json
import os
import pathlib

from launcher import checked_config


def materialize(config, source, aliases):
    c = checked_config(config)
    if c["mode"] != "local" or c["security"] != "partial" or not c["ws_coalescing"]:
        raise ValueError("matched SplitSnap all-local config required")
    src, dst = pathlib.Path(source["cache"]), pathlib.Path(c["cache_dir"])
    if (src == dst or pathlib.Path(c["run_root"]) not in src.parents or
            source["source_endpoint"] != c["minio_endpoint"] or
            aliases["run_id"] != c["run_id"] or aliases["materialized"] is not True or
            aliases["layout"] != "streams8-v1"):
        raise ValueError("source/config/alias identity differs")
    snapshot = source["snapshot"]
    if not snapshot.startswith("cold-aes-go-45000-45450-"):
        raise ValueError("only the fixed AES local point is in scope")
    rows = [a for a in aliases["aliases"] if a["endpoint"] == c["minio_endpoint"] and a["source_revision"] == snapshot]
    if len(rows) != 60 or {a["slot"] for a in rows} != set(range(60)):
        raise ValueError("60 matched aliases required")
    metadata = set(source["metadata_and_ws_files"])
    links = {k: k for k in metadata}
    for a in rows:
        if a["alias_revision"] != f"{snapshot}-{aliases['tag']}-{a['slot']}":
            raise ValueError("alias tag/slot differs")
        for e in a["source_entries"]:
            k = e["key"]
            if k not in metadata or not k.startswith(snapshot + "/") or (src / k).stat().st_size != e["size"]:
                raise ValueError("alias source not in verified local preparation")
            links[a["alias_revision"] + k[len(snapshot):]] = k
    native = list((src / "_chunks_zstd_v1_l3").rglob("*"))
    native = [p for p in native if p.is_file()]
    if len(native) != source["native_chunks"]:
        raise ValueError("local native-chunk inventory differs")
    for p in native:
        k = p.relative_to(src).as_posix()
        links[k] = k
    # All existence/size checks precede creating the new cache. Failed attempts
    # retain their explicit directory for inspection; there is no generic repair.
    for target, key in links.items():
        if pathlib.PurePosixPath(target).is_absolute() or ".." in pathlib.PurePosixPath(target).parts or not (src / key).is_file():
            raise ValueError("invalid/missing prepared file")
    dst.parent.mkdir(parents=True, exist_ok=True)
    dst.mkdir(exist_ok=False)
    for target, key in links.items():
        path = dst / target
        path.parent.mkdir(parents=True, exist_ok=True)
        os.link(src / key, path)
    return dict(cache=str(dst), source_cache=str(src), source_receipt=source,
                alias_tag=aliases["tag"], aliases=60, linked_files=len(links),
                definition="Same immutable compressed bytes and native/shared inputs; hardlinks prepare independent revision names, not a measured deduplication benefit.")


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("--config", type=pathlib.Path, required=True)
    p.add_argument("--source", type=pathlib.Path, required=True)
    p.add_argument("--aliases", type=pathlib.Path, required=True)
    p.add_argument("--report", type=pathlib.Path, required=True)
    a = p.parse_args()
    if a.report.exists():
        raise FileExistsError(a.report)
    result = materialize(*(json.loads(path.read_text()) for path in (a.config, a.source, a.aliases)))
    with a.report.open("x") as f:
        json.dump(result, f, indent=2)
        f.write("\n")
    print("LOCAL_ALIASES_COMPLETE", result["aliases"], result["linked_files"])


if __name__ == "__main__":
    main()
