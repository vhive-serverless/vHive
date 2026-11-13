#!/usr/bin/env bash
# Provision only the disposable hosted snapshot runner. Preserve its Docker CE.
set -euo pipefail

repo_dir=$(cd "$(dirname "$0")/../.." && pwd)
cd "$repo_dir"
: "${STACK_DIR:?Set the private CI build directory}"
export GOTOOLCHAIN=local
test "$(uname -m)" = x86_64
python3 -c 'import sys,tomllib; assert sys.version_info >= (3,11)'
# Check access as the installer user, not just through sudo. Fail rather than
# replace Docker CE/containerd.io with conflicting Ubuntu Docker packages.
docker version
docker buildx version

# Match the pinned runtime prepare-worker.sh build packages, excluding its
# docker.io/docker-buildx packages. net-tools supplies vHive's required route.
packages=(
    ca-certificates curl git rsync jq python3 build-essential pkg-config
    libseccomp-dev e2fsprogs squashfs-tools debootstrap dmsetup
    thin-provisioning-tools iproute2 iptables socat musl-tools
    protobuf-compiler bc flex bison libelf-dev libssl-dev dwarves net-tools
)
sudo -n apt-get update
sudo -n env DEBIAN_FRONTEND=noninteractive apt-get install \
    --yes --no-install-recommends --no-remove "${packages[@]}"
command -v route
docker version
docker buildx version

# build-stack.sh uses this exact STACK_DIR toolchain path. Take the official
# archive and checksum from the committed manifest instead of adding a new pin.
go_pin=$(python3 - <<'PY'
import json
import re
from pathlib import Path
go = json.loads(Path("configs/setup/firecracker-stack.json").read_text())["inputs"]["go"]
assert re.fullmatch(r"[0-9]+\.[0-9]+\.[0-9]+", go["version"])
assert go["archive"] == f'go{go["version"]}.linux-amd64.tar.gz'
assert re.fullmatch(r"[0-9a-f]{64}", go["sha256"])
print(go["version"], go["archive"], go["sha256"])
PY
)
read -r go_version go_archive go_sha256 <<< "$go_pin"
mkdir -p "$STACK_DIR/toolchains" "$STACK_DIR/downloads"
go_dir="$STACK_DIR/toolchains/go$go_version"
if [[ ! -x "$go_dir/bin/go" ]]; then
    archive="$STACK_DIR/downloads/$go_archive"
    curl --fail --location --retry 3 --output "$archive" "https://go.dev/dl/$go_archive"
    printf '%s  %s\n' "$go_sha256" "$archive" | sha256sum --check -
    staging=$(mktemp -d "$STACK_DIR/toolchains/.go.XXXXXXXX")
    trap 'rm -rf -- "$staging"' EXIT
    tar -xzf "$archive" -C "$staging"
    mv "$staging/go" "$go_dir"
    rmdir "$staging"
    trap - EXIT
fi
test "$("$go_dir/bin/go" version)" = "go version go$go_version linux/amd64"
"$go_dir/bin/go" version
if [[ -n "${GITHUB_PATH:-}" ]]; then
    printf '%s\n' "$go_dir/bin" >> "$GITHUB_PATH"
fi
dpkg-query -W -f='${Package}\t${Version}\n' "${packages[@]}"
