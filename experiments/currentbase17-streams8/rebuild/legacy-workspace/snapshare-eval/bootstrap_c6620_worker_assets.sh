#!/usr/bin/env bash
# Rebuild the SnapShare worker assets on a fresh c6620 without modifying the
# source xl170 worker. Transfers are resumable and validated before install.
set -euo pipefail

script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
workspace_dir=$(cd -- "$script_dir/.." && pwd)
source_env=${SOURCE_ENV:-$workspace_dir/colocation-infra/cluster/nodes.20260823.snapshare-xl170-4node.env}
target_env=${TARGET_ENV:-$workspace_dir/colocation-infra/cluster/nodes.20260824.snapshare-c6620-2node.env}
# shellcheck source=/dev/null
source "$source_env"
source_worker=${WORKER_NODES[0]}
# shellcheck source=/dev/null
source "$target_env"
target_worker=${WORKER_NODES[0]}
target_private_ip=${WORKER_PRIVATE_IPS[0]}
backend_private_ip=$BACKEND_PRIVATE_IP
vhive_dir=$workspace_dir/.dist/vhive-snap-benchmarking
ssh_cmd='ssh -A -oStrictHostKeyChecking=no -oServerAliveInterval=20 -oServerAliveCountMax=6'
rsync_ssh='ssh -A -oStrictHostKeyChecking=no -oServerAliveInterval=20 -oServerAliveCountMax=6'
temp_dir=$(mktemp -d)
trap 'rm -rf -- "$temp_dir"' EXIT

echo "[1/6] resume local vHive runtime assets"
rsync -a --partial --info=progress2 -e "$rsync_ssh" \
  "$vhive_dir/bin/" "$target_worker:~/vhive-snapshare/bin/"
rsync -a --partial -e "$rsync_ssh" \
  "$vhive_dir/cmd/relay/image_map.json" "$target_worker:~/vhive-snapshare/cmd/relay/image_map.json"
rsync -a --partial -e "$rsync_ssh" \
  "$vhive_dir/configs/firecracker-containerd/" "$target_worker:~/vhive-snapshare/configs/firecracker-containerd/"
rsync -a --partial -e "$rsync_ssh" \
  "$vhive_dir/configs/demux-snapshotter/" "$target_worker:~/vhive-snapshare/configs/demux-snapshotter/"
rsync -a --partial -e "$rsync_ssh" \
  "$vhive_dir/scripts/create_devmapper.sh" "$target_worker:~/vhive-snapshare/scripts/create_devmapper.sh"
rsync -a --partial -e "$rsync_ssh" \
  "$script_dir/restart_snapshare_xl170_worker.sh" "$target_worker:~/restart_snapshare_xl170_worker.sh"

echo "[2/6] copy provenance rootfs and vSwarm call assets from live source"
mkdir -p "$temp_dir/images" "$temp_dir/vswarm" "$temp_dir/cni"
rsync -a --partial --info=progress2 -e "$rsync_ssh" \
  "$source_worker:/users/Liquidz/images/" "$temp_dir/images/"
rsync -a --partial --info=progress2 -e "$rsync_ssh" \
  "$source_worker:/users/Liquidz/vswarm/" "$temp_dir/vswarm/"
rsync -a --partial --info=progress2 -e "$rsync_ssh" \
  "$source_worker:/opt/cni/bin/" "$temp_dir/cni/"
rsync -a --partial --info=progress2 -e "$rsync_ssh" \
  "$temp_dir/images/" "$target_worker:/users/Liquidz/images/"
rsync -a --partial --info=progress2 -e "$rsync_ssh" \
  "$temp_dir/vswarm/" "$target_worker:~/vswarm/"
rsync -a --partial --info=progress2 -e "$rsync_ssh" \
  "$temp_dir/cni/" "$target_worker:~/cni-bin/"

echo "[3/6] install Firecracker, containerd, CNI, and configs"
$ssh_cmd "$target_worker" 'bash -s' -- "$backend_private_ip" <<'REMOTE'
set -euo pipefail
backend_ip=$1
vhive=$HOME/vhive-snapshare
sudo mkdir -p /usr/local/bin /var/lib/firecracker-containerd/runtime \
  /var/lib/demux-snapshotter /etc/firecracker-containerd \
  /etc/demux-snapshotter /etc/containerd /opt/cni/bin
for binary in firecracker jailer containerd-shim-aws-firecracker \
  firecracker-containerd firecracker-ctr demux-snapshotter http-address-resolver; do
  sudo install -m 0755 "$vhive/bin/$binary" "/usr/local/bin/$binary"
done
sudo install -m 0644 "$vhive/bin/default-rootfs.img" \
  /var/lib/firecracker-containerd/runtime/default-rootfs.img
sudo install -m 0644 "$vhive/bin/vmlinux-5.10.186" \
  /var/lib/firecracker-containerd/runtime/hello-vmlinux.bin
sudo install -m 0644 "$vhive/configs/firecracker-containerd/config.toml" \
  /etc/firecracker-containerd/config.toml
sudo install -m 0644 "$vhive/configs/firecracker-containerd/firecracker-runtime.json" \
  /etc/containerd/firecracker-runtime.json
sudo install -m 0644 "$vhive/configs/demux-snapshotter/config.toml" \
  /etc/demux-snapshotter/config.toml
sudo install -m 0755 "$HOME"/cni-bin/* /opt/cni/bin/
chmod +x "$HOME/restart_snapshare_xl170_worker.sh" "$vhive/scripts/create_devmapper.sh"
sudo sysctl -w net.ipv4.ip_forward=1 >/dev/null
sudo modprobe kvm_intel
sudo modprobe dm_thin_pool
# The SnapShare networking code routes function traffic through the private
# backend interface.  Add a separate source-NAT rule for the one-time eStargz
# pull from an Internet registry on dual-homed CloudLab nodes.
public_iface=$(ip -o route show default | awk 'NR == 1 {print $5}')
if [[ -n $public_iface ]]; then
  sudo nft list table ip nat >/dev/null 2>&1 || sudo nft 'add table ip nat'
  sudo nft list chain ip nat SNAPSHARE_EGRESS >/dev/null 2>&1 || \
    sudo nft 'add chain ip nat SNAPSHARE_EGRESS { type nat hook postrouting priority srcnat; policy accept; }'
  sudo nft flush chain ip nat SNAPSHARE_EGRESS
  sudo nft add rule ip nat SNAPSHARE_EGRESS ip saddr 172.16.0.0/12 \
    oifname "$public_iface" counter masquerade
fi
if ! grep -q 'docker-registry.registry.svc.cluster.local' /etc/hosts; then
  printf '%s %s\n' "$backend_ip" docker-registry.registry.svc.cluster.local | sudo tee -a /etc/hosts >/dev/null
fi
REMOTE

echo "[4/6] create devmapper thin pool"
$ssh_cmd "$target_worker" '~/vhive-snapshare/scripts/create_devmapper.sh'

echo "[5/6] validate exact runtime and provenance assets"
local_relay_sha=$(sha256sum "$vhive_dir/bin/relay" | awk '{print $1}')
local_rootfs_sha=$(sha256sum "$vhive_dir/bin/default-rootfs.img" | awk '{print $1}')
source_provenance_sha=$($ssh_cmd "$source_worker" \
  'sha256sum /users/Liquidz/images/rootfs.tar' | awk '{print $1}')
$ssh_cmd "$target_worker" 'bash -s' -- \
  "$local_relay_sha" "$local_rootfs_sha" "$source_provenance_sha" "$target_private_ip" <<'REMOTE'
set -euo pipefail
relay_sha=$1
rootfs_sha=$2
provenance_sha=$3
private_ip=$4
test "$(sha256sum ~/vhive-snapshare/bin/relay | awk '{print $1}')" = "$relay_sha"
test "$(sha256sum /var/lib/firecracker-containerd/runtime/default-rootfs.img | awk '{print $1}')" = "$rootfs_sha"
test "$(sha256sum /users/Liquidz/images/rootfs.tar | awk '{print $1}')" = "$provenance_sha"
test -c /dev/kvm
test -e /dev/mapper/fc-dev-thinpool
test "$(getent hosts docker-registry.registry.svc.cluster.local | awk '{print $1}')" = 10.0.1.2
ip -o route get 10.0.1.2 | grep -q "src $private_ip"
printf 'WORKER_ASSET_VALIDATION_PASS relay=%s rootfs=%s provenance=%s\n' \
  "$relay_sha" "$rootfs_sha" "$provenance_sha"
REMOTE

echo "[6/6] complete"
