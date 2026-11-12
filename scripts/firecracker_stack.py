#!/usr/bin/env python3
"""Build, verify and install vHive's co-versioned Firecracker stack."""

import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import tempfile


ROOT = Path(__file__).resolve().parents[1]
DEFAULT_MANIFEST = ROOT / "configs/setup/firecracker-stack.json"
COMMANDS = ("validate", "prepare", "build", "install", "verify", "activate")


def require_host_networking():
    if not shutil.which("route"):
        raise ValueError("vHive networking requires route in PATH; run prepare or install --prepare to install net-tools")


def prepare_host_networking():
    # These vHive prerequisites are additional to the runtime build toolchain.
    if not shutil.which("route"):
        run("sudo", "-n", "apt-get", "update")
        run("sudo", "-n", "env", "DEBIAN_FRONTEND=noninteractive", "apt-get",
            "install", "--yes", "--no-install-recommends", "net-tools")
    require_host_networking()


def run(*args, cwd=None, env=None, capture=False):
    return subprocess.run(args, cwd=cwd, env=env, check=True, text=True,
                          stdout=subprocess.PIPE if capture else None).stdout


def digest(path):
    if not os.access(path, os.R_OK):
        return run("sudo", "-n", "sha256sum", "--", str(path), capture=True).split()[0]
    result = hashlib.sha256()
    with open(path, "rb") as source:
        for chunk in iter(lambda: source.read(4 * 1024 * 1024), b""):
            result.update(chunk)
    return result.hexdigest()


def require_subset(expected, actual, location="inputs"):
    if isinstance(expected, dict):
        if not isinstance(actual, dict):
            raise ValueError(f"{location}: expected an object")
        for key, value in expected.items():
            if key not in actual:
                raise ValueError(f"{location}.{key}: missing pinned input")
            require_subset(value, actual[key], f"{location}.{key}")
    elif expected != actual:
        raise ValueError(f"{location}: expected {expected!r}, found {actual!r}")


def load_manifest(path):
    manifest = json.loads(path.read_text())
    if manifest.get("schema_version") != 1:
        raise ValueError("unsupported Firecracker stack manifest version")
    runtime = manifest["runtime"]
    if not re.fullmatch(r"[0-9a-f]{40}", runtime["revision"]):
        raise ValueError("runtime revision must be a complete immutable Git commit")
    if runtime["repository"] != "https://github.com/vhive-serverless/firecracker-containerd.git":
        raise ValueError("runtime must come from the vHive organization")
    if set(manifest["artifacts"]) != {"firecracker", "jailer", "runtime", "ctr", "shim", "runc", "kernel", "rootfs", "demux", "resolver"}:
        raise ValueError("manifest must cover the complete runtime, guest and snapshotter stack")
    for artifact in manifest["artifacts"].values():
        for key in ("source", "installed"):
            path = Path(artifact[key])
            if path.is_absolute() or ".." in path.parts:
                raise ValueError(f"artifact {key} must be a relative path within the stack")
    expected = manifest["inputs"]
    if expected["firecracker"]["commit"] != expected["firecracker"]["base_commit"]:
        raise ValueError("Firecracker must use unmodified upstream source")
    if expected["firecracker"]["binary_source"] != "official GitHub release":
        raise ValueError("Firecracker must use a verified official release")
    return manifest


def deployment_revision(manifest):
    revision = os.environ.get("VHIVE_RUNTIME_REVISION", manifest["runtime"]["revision"])
    if not re.fullmatch(r"[0-9a-f]{40}", revision):
        raise ValueError("VHIVE_RUNTIME_REVISION must be a complete Git commit")
    if not manifest["runtime"]["ready"] and "VHIVE_RUNTIME_REVISION" not in os.environ:
        raise ValueError("runtime pin is pending; set an exact VHIVE_RUNTIME_REVISION for development or finalize the manifest")
    return revision


def check_checkout(source, revision, manifest):
    actual = run("git", "rev-parse", "HEAD", cwd=source, capture=True).strip()
    if actual != revision:
        raise ValueError(f"runtime checkout is {actual}, expected {revision}")
    if run("git", "status", "--porcelain", cwd=source, capture=True).strip():
        raise ValueError("runtime build requires a clean committed checkout")
    lock = json.loads((source / "docs/modernization/versions.json").read_text())
    require_subset(manifest["inputs"], lock["selected_inputs"])


def get_checkout(stack, revision):
    supplied = os.environ.get("VHIVE_RUNTIME_CHECKOUT")
    if supplied:
        return Path(supplied).expanduser().resolve()
    source = stack / "sources" / revision
    if not source.exists():
        source.parent.mkdir(parents=True, exist_ok=True)
        source.mkdir()
        run("git", "init", "--quiet", str(source))
        run("git", "remote", "add", "origin", "https://github.com/vhive-serverless/firecracker-containerd.git", cwd=source)
    if not (source / "docs/modernization/versions.json").exists():
        run("git", "fetch", "--depth=1", "origin", revision, cwd=source)
        run("git", "checkout", "--detach", revision, cwd=source)
    return source


def verify_install(prefix, revision, manifest):
    receipt = json.loads((prefix / "vhive-stack-receipt.json").read_text())
    require_subset({"runtime_revision": revision, "manifest": manifest}, receipt, "receipt")
    expected_names = {item["installed"] for item in manifest["artifacts"].values()}
    if set(receipt["installed_sha256"]) != expected_names:
        raise ValueError("installation receipt does not cover exactly the manifest artifacts")
    for name, expected in receipt["installed_sha256"].items():
        if digest(prefix / name) != expected:
            raise ValueError(f"installed artifact differs from receipt: {name}")
    return receipt


def make_receipt(source, prefix, revision, manifest):
    built = source / "bin/receipts/build"
    if (built / "runtime-revision").read_text().strip() != revision:
        raise ValueError("runtime build receipt belongs to a different revision")
    installed = {}
    for item in manifest["artifacts"].values():
        expected = digest(source / item["source"])
        if digest(prefix / item["installed"]) != expected:
            raise ValueError(f"installed artifact differs from selected build: {item['installed']}")
        installed[item["installed"]] = expected
    return {"schema_version": 1, "runtime_revision": revision, "manifest": manifest,
            "runtime_lock_sha256": digest(source / "docs/modernization/versions.json"),
            "build_receipt_sha256": digest(built / "binaries.sha256"),
            "guest_agent_sha256": digest(source / "agent/agent"),
            "installed_sha256": installed}


def write_privileged(source, destination, mode="644"):
    run("sudo", "-n", "install", "-D", "-m", mode, str(source), str(destination))


def activate(prefix, revision, manifest, pool):
    # Verify all artifacts before making the installation visible to vHive.
    verify_install(prefix, revision, manifest)
    require_host_networking()
    if not re.fullmatch(r"[A-Za-z0-9_.+-]+", pool):
        raise ValueError("invalid devmapper pool name")
    with tempfile.TemporaryDirectory(prefix="vhive-stack-config-") as directory:
        temporary = Path(directory)
        config = (ROOT / "configs/firecracker-containerd/config.toml").read_text().replace("fc-dev-thinpool", pool)
        (temporary / "config.toml").write_text(config)
        template = json.loads((ROOT / "configs/firecracker-containerd/firecracker-runtime.json").read_text())

        def render(value):
            if isinstance(value, dict):
                return {key: render(item) for key, item in value.items()}
            if isinstance(value, list):
                return [render(item) for item in value]
            if isinstance(value, str):
                return value.replace("${STACK_PREFIX}", str(prefix))
            return value

        runtime = render(template)
        (temporary / "firecracker-runtime.json").write_text(json.dumps(runtime, indent=2) + "\n")
        write_privileged(temporary / "config.toml", "/etc/firecracker-containerd/config.toml")
        write_privileged(temporary / "firecracker-runtime.json", "/etc/containerd/firecracker-runtime.json")
    write_privileged(ROOT / "configs/demux-snapshotter/config.toml", "/etc/demux-snapshotter/config.toml")
    run("sudo", "-n", "mkdir", "-p", "/var/lib/demux-snapshotter", "/var/lib/firecracker-containerd/runtime")
    run("sudo", "-n", "mkdir", "-p", "/fccd")
    run("sudo", "-n", "chmod", "1777", "/fccd")
    for name in ("firecracker", "jailer", "firecracker-containerd", "firecracker-ctr",
                 "containerd-shim-aws-firecracker", "demux-snapshotter", "http-address-resolver"):
        run("sudo", "-n", "ln", "-sfn", str(prefix / "bin" / name), "/usr/local/bin/" + name)
    print(f"Activated verified vHive stack {revision} from {prefix}")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("command", choices=COMMANDS)
    parser.add_argument("--manifest", type=Path, default=DEFAULT_MANIFEST)
    parser.add_argument("--prepare", action="store_true", help="provision runtime build and vHive host networking prerequisites")
    parser.add_argument("--activate", action="store_true", help="configure vHive and link its commands after a verified install")
    parser.add_argument("--pool", default=os.environ.get("FICD_DM_POOL", "fc-dev-thinpool"))
    args = parser.parse_args()
    manifest = load_manifest(args.manifest)
    if args.command == "validate":
        print(f"Validated stack manifest; runtime pin ready={manifest['runtime']['ready']}")
        return
    revision = deployment_revision(manifest)
    stack = Path(os.environ.get("STACK_DIR", "~/.local/share/vhive/firecracker-stack")).expanduser().resolve()
    prefix = Path(os.environ.get("INSTALL_DIR", str(stack / "install" / revision[:12]))).expanduser().resolve()
    if args.command in ("verify", "activate"):
        verify_install(prefix, revision, manifest)
        if args.command == "activate":
            activate(prefix, revision, manifest, args.pool)
        print(f"Verified stack installation {prefix}")
        return
    if args.command == "prepare" or args.prepare:
        # Repair host prerequisites even when an existing receipt avoids rebuilds.
        prepare_host_networking()
    elif args.activate:
        require_host_networking()
    source = get_checkout(stack, revision)
    check_checkout(source, revision, manifest)
    environment = dict(os.environ, STACK_DIR=str(stack), INSTALL_DIR=str(prefix), GOWORK="off")
    # The runtime's scripts independently enforce their clean source and input lock.
    environment.pop("ALLOW_DIRTY", None)
    if args.command == "prepare" or (args.prepare and not (prefix / "vhive-stack-receipt.json").exists()):
        run("bash", "tools/modernize/prepare-worker.sh", cwd=source, env=environment)
    if args.command == "prepare":
        return
    if args.command == "install" and (prefix / "vhive-stack-receipt.json").exists():
        verify_install(prefix, revision, manifest)
    else:
        run("bash", "tools/modernize/build-stack.sh", cwd=source, env=environment)
        if args.command == "build":
            return
        run("bash", "tools/modernize/install-stack.sh", cwd=source, env=environment)
        receipt = make_receipt(source, prefix, revision, manifest)
        with tempfile.TemporaryDirectory(prefix="vhive-stack-receipt-") as directory:
            staged = Path(directory) / "vhive-stack-receipt.json"
            staged.write_text(json.dumps(receipt, indent=2) + "\n")
            write_privileged(staged, prefix / staged.name)
    verify_install(prefix, revision, manifest)
    if args.activate:
        activate(prefix, revision, manifest, args.pool)
    print(f"Installed verified stack {revision} at {prefix}")


if __name__ == "__main__":
    try:
        main()
    except (ValueError, KeyError, OSError, subprocess.CalledProcessError) as error:
        raise SystemExit(f"Firecracker stack setup failed: {error}") from error
