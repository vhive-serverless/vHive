#!/usr/bin/env python3
"""Accept the installed Step 2 stack through vHive's legacy snapshot workflow.

Run only on a dedicated, provisioned KVM worker with an existing empty devmapper
pool and no daemon at /run/firecracker-containerd/containerd.sock. This script
starts and stops its own daemon; it does not reset pools, networks or services.

First finalize the committed runtime replacement in go.mod/go.sum and the stack
manifest (including its SDK input). Install/activate that exact stack with:

  VHIVE_RUNTIME_REVISION=<full-step-2-commit> INSTALL_DIR=<private-prefix> \
    scripts/setup_firecracker_containerd.sh --pool <existing-test-pool>

Then run this script with sudo, --prefix, --runtime-revision and --output. The
default requires a finalized ready=true manifest; --candidate permits a pending
manifest but still requires the committed Go dependency and installed runtime to
match the supplied revision. Candidate success is never final acceptance.
"""

import argparse
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import sys
import time


ROOT = Path(__file__).resolve().parents[1]
TESTS = ("TestPauseSnapResume", "TestSnapshotConcurrentClones",
         "TestSnapshotSourceMemoryAndDiskIsolation")


def run(*args, env=None, capture=False):
    return subprocess.run(args, cwd=ROOT, env=env, text=True, check=True,
                          stdout=subprocess.PIPE if capture else None).stdout


def sha256(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def module_origin(go, module, expected, env):
    selected = json.loads(run(go, "list", "-mod=readonly", "-m", "-json", module,
                              env=env, capture=True))
    replacement = selected.get("Replace", {})
    if not replacement.get("Version") or not replacement.get("Path", "").startswith("github.com/vhive-serverless/"):
        raise ValueError(f"{module}: expected a published vHive module, not a local replacement")
    downloaded = json.loads(run(go, "mod", "download", "-json",
                               replacement["Path"] + "@" + replacement["Version"],
                               env=env, capture=True))
    if downloaded.get("Origin", {}).get("Hash") != expected:
        raise ValueError(f"{module}: dependency does not resolve to expected commit {expected}")
    return {"selected": selected, "downloaded": downloaded}


def main():
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("--prefix", required=True, type=Path, help="verified, activated INSTALL_DIR")
    parser.add_argument("--runtime-revision", required=True, help="full accepted Step 2 Git commit")
    parser.add_argument("--output", required=True, type=Path, help="new receipt/log directory outside the checkout")
    parser.add_argument("--image", help="immutable image@sha256:digest; defaults to the runtime input lock's vHive function image")
    parser.add_argument("--go", default=shutil.which("go"), help="Go 1.26.11 executable")
    parser.add_argument("--candidate", action="store_true", help="validate a pending pin without claiming final acceptance")
    args = parser.parse_args()
    revision = args.runtime_revision
    if not re.fullmatch(r"[0-9a-f]{40}", revision):
        raise ValueError("--runtime-revision must be a full immutable commit")
    if not args.go:
        raise ValueError("Go is required")
    if args.image and not re.fullmatch(r".+@sha256:[0-9a-f]{64}", args.image):
        raise ValueError("--image must select an immutable sha256 digest")
    if os.geteuid() != 0 or not os.access("/dev/kvm", os.R_OK | os.W_OK):
        raise ValueError("run as root on an isolated KVM worker")
    prefix, output = args.prefix.resolve(), args.output.resolve()
    if output.is_relative_to(ROOT):
        raise ValueError("--output must be outside the source checkout")
    if output.exists():
        raise ValueError("--output must be a new directory")
    if run("git", "status", "--porcelain", capture=True).strip():
        raise ValueError("vHive acceptance requires a clean, committed checkout")

    sys.dont_write_bytecode = True
    spec = importlib.util.spec_from_file_location("firecracker_stack", ROOT / "scripts/firecracker_stack.py")
    stack = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(stack)
    stack.require_host_networking()
    manifest = stack.load_manifest(stack.DEFAULT_MANIFEST)
    if not args.candidate and (not manifest["runtime"]["ready"] or manifest["runtime"]["revision"] != revision):
        raise ValueError("final acceptance requires a finalized manifest matching --runtime-revision")
    receipt = stack.verify_install(prefix, revision, manifest)
    config = json.loads(Path("/etc/containerd/firecracker-runtime.json").read_text())
    expected_paths = {"firecracker_binary_path": prefix / "bin/firecracker",
                      "kernel_image_path": prefix / "share/runtime/default-vmlinux.bin",
                      "root_drive": prefix / "share/runtime/default-rootfs.img"}
    for key, expected in expected_paths.items():
        if Path(config[key]).resolve() != expected:
            raise ValueError(f"activated {key} does not select the verified installation")
    if Path(config["jailer"]["runc_binary_path"]).resolve() != prefix / "bin/runc":
        raise ValueError("activated runc does not select the verified installation")
    version = run(str(prefix / "bin/firecracker"), "--version", capture=True)
    if version.splitlines()[0] != "Firecracker v1.18.0":
        raise ValueError(f"unexpected Firecracker version: {version}")
    socket = Path("/run/firecracker-containerd/containerd.sock")
    if socket.exists() or Path(str(socket) + ".ttrpc").exists():
        raise ValueError("a containerd socket already exists; use an idle dedicated worker")
    # The activated configuration selects the pool. Never silently create/reset it.
    import tomllib
    daemon_config = Path("/etc/firecracker-containerd/config.toml")
    settings = tomllib.loads(daemon_config.read_text())
    pool = settings["plugins"]["io.containerd.snapshotter.v1.devmapper"]["pool_name"]
    run("dmsetup", "info", pool, capture=True)
    if settings["plugins"]["io.containerd.server.v1.grpc"]["address"] != str(socket):
        raise ValueError("activated containerd address differs from vHive's endpoint")

    # Disable workspace and command-line dependency substitutions. The committed
    # module graph itself must compile and resolve to the installed revision.
    environment = dict(os.environ, GOWORK="off", GOFLAGS="", GOTOOLCHAIN="local")
    environment["PATH"] = str(prefix / "bin") + os.pathsep + environment.get("PATH", os.defpath)
    environment.pop("VHIVE_RUNTIME_REVISION", None)
    runtime_module = module_origin(args.go, "github.com/firecracker-microvm/firecracker-containerd", revision, environment)
    sdk_module = module_origin(args.go, "github.com/firecracker-microvm/firecracker-go-sdk", manifest["inputs"]["sdk"]["commit"], environment)
    runtime_lock = Path(runtime_module["downloaded"]["Dir"]) / "docs/modernization/versions.json"
    selected_inputs = json.loads(runtime_lock.read_text())["selected_inputs"]
    stack.require_subset(manifest["inputs"], selected_inputs)
    if sha256(runtime_lock) != receipt["runtime_lock_sha256"]:
        raise ValueError("compiled runtime module and installation have different input locks")
    image = args.image or selected_inputs.get("vhive", {}).get("function_image")
    if not image or not re.fullmatch(r".+@sha256:[0-9a-f]{64}", image):
        raise ValueError("runtime lock must pin vhive.function_image or --image must supply an immutable digest")
    output.mkdir(parents=True)
    provenance = {"vhive_revision": run("git", "rev-parse", "HEAD", capture=True).strip(),
                  "runtime_revision": revision, "candidate": args.candidate, "image": image,
                  "go_version": run(args.go, "version", capture=True).strip(),
                  "firecracker_version": version.strip(), "pool": pool,
                  "runtime_module": runtime_module, "sdk_module": sdk_module,
                  "installation": receipt,
                  "source_sha256": {name: sha256(ROOT / name) for name in (
                      "go.mod", "go.sum", "scripts/accept_legacy_snapshot.py",
                      "ctriface/snapshot_continuity_test.go", "ctriface/snapshot_clones_test.go")},
                  "config_sha256": {str(path): sha256(path) for path in (
                      daemon_config, Path("/etc/containerd/firecracker-runtime.json"))},
                  "tests": list(TESTS), "result": "not run"}
    record = output / "acceptance.json"
    record.write_text(json.dumps(provenance, indent=2) + "\n")
    binary = output / "ctriface.test"
    with (output / "compile.log").open("w") as log:
        subprocess.run([args.go, "test", "-mod=readonly", "-c", "-o", str(binary), "./ctriface"],
                       cwd=ROOT, env=environment, stdout=log, stderr=subprocess.STDOUT, check=True)
    (output / "test-binary-buildinfo.txt").write_text(run(args.go, "version", "-m", str(binary), capture=True))
    command = [str(binary), "-test.v", "-test.count=1", "-test.timeout=15m",
               "-test.run=^(" + "|".join(TESTS) + ")$", "-ss=devmapper", "-upf=false", "-img=" + image]
    provenance["command"] = command
    daemon = None
    try:
        with (output / "daemon.log").open("w") as log:
            daemon = subprocess.Popen([str(prefix / "bin/firecracker-containerd"), "--config", str(daemon_config)],
                                      env=environment, stdout=log, stderr=subprocess.STDOUT)
            deadline = time.monotonic() + 30
            while not socket.exists():
                if daemon.poll() is not None or time.monotonic() > deadline:
                    raise RuntimeError("containerd failed to become ready; inspect daemon.log")
                time.sleep(0.1)
            with (output / "tests.log").open("w") as tests:
                subprocess.run(command, cwd=ROOT, env=environment, stdout=tests,
                               stderr=subprocess.STDOUT, check=True, timeout=930)
            (output / "images.txt").write_text(run(str(prefix / "bin/firecracker-ctr"), "--address", str(socket),
                                                   "--namespace", "firecracker-containerd", "images", "ls", capture=True))
            # Go exits successfully when a selected test skips; require all three
            # actual VM cases to pass, never merely a zero process exit status.
            logs = (output / "tests.log").read_text()
            for name in TESTS:
                if not re.search(r"^--- PASS: " + re.escape(name) + r" \(", logs, re.MULTILINE):
                    raise RuntimeError(f"required VM test did not pass: {name}")
        provenance["result"] = "candidate pass" if args.candidate else "pass"
    except BaseException as error:
        provenance["result"] = "failed"
        provenance["error"] = str(error)
        raise
    finally:
        if daemon is not None and daemon.poll() is None:
            daemon.terminate()
            try:
                daemon.wait(timeout=30)
            except subprocess.TimeoutExpired:
                daemon.kill()
                daemon.wait()
        record.write_text(json.dumps(provenance, indent=2) + "\n")
    marker = "VHIVE_LEGACY_SNAPSHOT_CANDIDATE_OK" if args.candidate else "VHIVE_LEGACY_SNAPSHOT_ACCEPTANCE_OK"
    print(f"{marker} receipt={record}")


if __name__ == "__main__":
    main()
