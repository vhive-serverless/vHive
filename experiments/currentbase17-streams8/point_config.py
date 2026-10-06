"""Resolve one accepted matrix row into an isolated worker relay configuration."""
import pathlib
import re
import argparse
import json

from launcher import checked_config


SYSTEMS = {"chunks-128k-zstd3", "pages-4k-zstd3", "ws-zstd3", "no-image-zstd3",
           "splitsnap-zstd3", "full-dedup-zstd3"}


def point_config(plan, system, profile, attempt, mode="remote"):
    if system not in SYSTEMS or not re.fullmatch(r"[a-z0-9-]+", attempt):
        raise ValueError("invalid system/attempt")
    if {s["id"] for s in plan["matrix"]["systems"]} != SYSTEMS:
        raise ValueError("matrix system set differs")
    matches = [w for w in plan["workloads"]["workloads"] if w["profile"] == profile]
    if len(matches) != 1:
        raise ValueError("profile not unique")
    w = matches[0]
    s = next(s for s in plan["matrix"]["systems"] if s["id"] == system)
    override = w["corpus_overrides"][system]
    jobs = [j for j in plan["jobs"] if j["corpus_id"] == override["corpus_id"]]
    if len(jobs) != 1 or jobs[0]["destination"] != f"{plan['backend']}:{override['port']}":
        raise ValueError("job/endpoint differs")
    if mode == "local" and (system != "splitsnap-zstd3" or profile != "aes-go-45000-45450"):
        raise ValueError("only matched SplitSnap AES all-local is in this matrix")
    point_id = f"{system}-{profile.replace('_', '-')}-{attempt}-{mode}"
    root = pathlib.PurePosixPath("/users/Liquidz/streams8") / plan["run_id"]
    point = root / "points" / point_id
    c = dict(run_id=plan["run_id"], point_id=point_id, run_root=str(root),
             cache_dir=str(point / "snapshots"), scratch_dir=str(point / "scratch"),
             log_dir=str(point / "logs"), relay_binary=str(root / "bin/relay-streams8"),
             relay_cwd=str(root / "relay"), images_dir="/users/Liquidz/images",
             relay_endpoint="10.0.1.1:8090", ws_cache_endpoint="10.0.1.1:8091", network_name_prefix="sstr",
             veth_prefix="172.30", clone_prefix="172.31", host_iface="enp23s0f0np0",
             dns="10.0.1.2", minio_endpoint=jobs[0]["destination"], security=s["security"],
             vm_mib=w["vm_mib"], chunk_size=s["chunk_size"], ws_coalescing=s["ws_coalescing"],
             base_snap=s["base_snap"], mode=mode, clean=mode == "remote")
    return checked_config(c)


if __name__ == "__main__":
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("--plan", type=pathlib.Path, required=True)
    p.add_argument("--system", required=True)
    p.add_argument("--profile", required=True)
    p.add_argument("--attempt", required=True)
    p.add_argument("--mode", default="remote", choices=("remote", "local"))
    p.add_argument("--relay-binary", default="")
    p.add_argument("--output", type=pathlib.Path, required=True)
    a = p.parse_args()
    config = point_config(json.loads(a.plan.read_text()), a.system, a.profile, a.attempt, a.mode)
    if a.relay_binary:
        config["relay_binary"] = a.relay_binary
        config = checked_config(config)
    with a.output.open("x") as f:
        json.dump(config, f, indent=2)
        f.write("\n")
