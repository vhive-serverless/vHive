#!/usr/bin/env bash
# Build an explicitly authorized isolated c6620 platform used for Zstd streaming and
# compressed-chunk development.  This entry point is intentionally separate
# from bootstrap_c6620_fresh_two_node.sh so the live Full Dedup defaults and
# services remain untouched.
set -euo pipefail

script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
workspace_dir=$(cd -- "$script_dir/.." && pwd)
nodes_env=${NODES_ENV:-$workspace_dir/colocation-infra/cluster/nodes.20260827.zstd-streaming-c6620-2node.env}
# shellcheck source=/dev/null
source "$nodes_env"

recovery_dir=${RECOVERY_DIR:-$workspace_dir/IAA单节点-保存/恢复/node-handoff-20260822}
vhive_dir=${VHIVE_DIR:-$workspace_dir/.dist/vhive-zstd-streaming}
runtime_asset_dir=${RUNTIME_ASSET_DIR:-$workspace_dir/.dist/vhive-snap-benchmarking}
backend_device=${BACKEND_DEVICE:-/dev/nvme1n1}
backend_device_serial=${BACKEND_DEVICE_SERIAL:?BACKEND_DEVICE_SERIAL must be set by the nodes env}
backend_mount=${BACKEND_MOUNT:-/mnt/snapshare-zstd-streaming}
cpu_state_dir=${CPU_STATE_DIR:-/users/Liquidz/cpu-baseline-zstd-streaming-before-2p1ghz}
worker=${WORKER_NODES[0]}
worker_private_ip=${WORKER_PRIVATE_IPS[0]}
loader=${LOADER_NODE:-$BACKEND_NODE}
loader_private_ip=${LOADER_PRIVATE_IP:-$BACKEND_PRIVATE_IP}

remote_bootstrap_dir=${REMOTE_BOOTSTRAP_DIR:-snapshare-zstd-bootstrap}
remote_vhive_dir=${REMOTE_VHIVE_DIR:-vhive-zstd-streaming}
remote_runtime_dir=${REMOTE_RUNTIME_DIR:-snapshare-zstd-runtime}
remote_vswarm_dir=${REMOTE_VSWARM_DIR:-vswarm-zstd-streaming}

minio_host=${SNAPSHARE_MINIO_HOST:-$BACKEND_PRIVATE_IP}
minio_port=${SNAPSHARE_MINIO_PORT:-9000}
mongo_host=${SNAPSHARE_MONGODB_HOST:-$BACKEND_PRIVATE_IP}
mongo_port=${SNAPSHARE_MONGODB_PORT:-27017}
registry_host=${SNAPSHARE_REGISTRY_HOST:-$BACKEND_PRIVATE_IP}
registry_port=${SNAPSHARE_REGISTRY_PORT:-5000}
dns_port=${SNAPSHARE_DNS_PORT:-53}
dns_nameservers=${SNAPSHARE_DNS_NAMESERVERS:-$BACKEND_PRIVATE_IP}
provenance_http_port=${SNAPSHARE_PROVENANCE_HTTP_PORT:-18081}
relay_port=${SNAPSHARE_RELAY_PORT:-8080}

minio_container=${SNAPSHARE_MINIO_CONTAINER:-snapshare-zstd-minio}
mongo_container=${SNAPSHARE_MONGODB_CONTAINER:-snapshare-zstd-mongodb}
registry_container=${SNAPSHARE_REGISTRY_CONTAINER:-snapshare-zstd-registry}
mongo_db_volume=${SNAPSHARE_MONGODB_DB_VOLUME:-snapshare-zstd-mongodb-db}
mongo_config_volume=${SNAPSHARE_MONGODB_CONFIG_VOLUME:-snapshare-zstd-mongodb-config}
dns_tmux_session=${SNAPSHARE_DNS_TMUX_SESSION:-snapshare_zstd_dns}
minio_data_dir=${SNAPSHARE_MINIO_DATA_DIR:-$backend_mount/minio-main}
registry_data_dir=${SNAPSHARE_REGISTRY_DATA_DIR:-$backend_mount/registry}

expected_relay_sha=${EXPECTED_RELAY_SHA:-}
expected_vswarm_relay_sha=${EXPECTED_VSWARM_RELAY_SHA:-f3b331f67c8fdab95411fc861a261e56ae7df55fa9cd3e5fb9c4b9db5f93d9a0}
expected_guest_rootfs_sha=${EXPECTED_GUEST_ROOTFS_SHA:-2009784c0896efc4e66bffce6f2a74a33f2c5d2aaad486e1b38b80dfd420a902}

service_archive=$recovery_dir/images/docker-service-images.tar.zst
mongo_dump=$recovery_dir/mongodb/snapshare-mongodb.archive.gz
provenance_archive=$recovery_dir/packages/function-provenance-images.tar.zst
function_archive=$script_dir/artifacts/function-images/20260818-rebuild-f268d11/image-rotate-go.restore-reconnect-f268d11-r2-20260818.esgz.oci.tar
relay_binary=${ZSTD_RELAY_BINARY:-$vhive_dir/bin/relay}
vswarm_relay_binary=$workspace_dir/vSwarm/tools/relay/server
guest_rootfs=$script_dir/artifacts/runtime/default-rootfs.plain-http-registry-20260826.img
function_tag=restore-reconnect-f268d11-r2-20260818-esgz

ssh_opts=(-A -oStrictHostKeyChecking=no -oServerAliveInterval=20 -oServerAliveCountMax=6)
rsync_ssh='ssh -A -oStrictHostKeyChecking=no -oServerAliveInterval=20 -oServerAliveCountMax=6'

log() { printf '%s %s\n' "$(date -Is)" "$*"; }
die() { printf 'ERROR: %s\n' "$*" >&2; exit 1; }

require_file() {
  [[ -f $1 ]] || die "missing local artifact: $1"
}

require_safe_name() {
  [[ $2 =~ ^[a-zA-Z0-9][a-zA-Z0-9_.-]*$ ]] || die "invalid $1: $2"
}

require_port() {
  [[ $2 =~ ^[1-9][0-9]*$ && $2 -le 65535 ]] || die "invalid $1: $2"
}

is_forbidden_node() {
  local candidate=$1 forbidden
  for forbidden in "${FORBIDDEN_NODES[@]}"; do
    [[ $candidate != "$forbidden" ]] || return 0
  done
  return 1
}

is_forbidden_ip() {
  local candidate=$1 forbidden
  for forbidden in "${FORBIDDEN_PRIVATE_IPS[@]}"; do
    [[ $candidate != "$forbidden" ]] || return 0
  done
  return 1
}

verify_environment_isolation() {
  [[ -n ${AUTHORIZED_EXPERIMENT_ID:-} && ${EXPERIMENT_ID:-} == "$AUTHORIZED_EXPERIMENT_ID" ]] \
    || die "unexpected EXPERIMENT_ID: ${EXPERIMENT_ID:-unset}"
  [[ ${RESULT_FAMILY:-} == zstd-streaming ]] || die "unexpected RESULT_FAMILY"
  [[ ${RESULT_ROOT_RELATIVE:-} == snapshare-eval/results/zstd-streaming ]] \
    || die "result root must remain in the zstd-streaming namespace"
  [[ -n ${AUTHORIZED_CONTROL_NODE:-} && $CONTROL_NODE == "$AUTHORIZED_CONTROL_NODE" ]] \
    || die "worker/control does not match the authorized node"
  [[ -n ${AUTHORIZED_BACKEND_NODE:-} && $BACKEND_NODE == "$AUTHORIZED_BACKEND_NODE" ]] \
    || die "backend does not match the authorized node"
  [[ -n ${AUTHORIZED_CONTROL_PRIVATE_IP:-} && $CONTROL_PRIVATE_IP == "$AUTHORIZED_CONTROL_PRIVATE_IP" ]] \
    || die "worker/control private IP is not authorized"
  [[ -n ${AUTHORIZED_BACKEND_PRIVATE_IP:-} && $BACKEND_PRIVATE_IP == "$AUTHORIZED_BACKEND_PRIVATE_IP" ]] \
    || die "backend private IP is not authorized"
  [[ $worker == "$CONTROL_NODE" && $worker_private_ip == "$CONTROL_PRIVATE_IP" ]] \
    || die "WORKER_NODES/WORKER_PRIVATE_IPS do not resolve to the authorized control"
  [[ $worker != "$BACKEND_NODE" && $worker_private_ip != "$BACKEND_PRIVATE_IP" ]] \
    || die "worker and backend must be distinct"
  if [[ ${REQUIRE_DISTINCT_LOADER:-0} == 1 ]]; then
    [[ $loader != "$worker" && $loader != "$BACKEND_NODE" ]] \
      || die "loader must be distinct from worker and backend"
    [[ $loader_private_ip != "$worker_private_ip" && $loader_private_ip != "$BACKEND_PRIVATE_IP" ]] \
      || die "loader private IP must be distinct"
    [[ -n ${LOADER_HOSTNAME:-} ]] || die "LOADER_HOSTNAME is required for a distinct loader"
  elif [[ ${REQUIRE_DISTINCT_LOADER:-0} != 0 ]]; then
    die "REQUIRE_DISTINCT_LOADER must be 0 or 1"
  fi

  local endpoint
  for endpoint in "$worker" "$BACKEND_NODE" "$LOADER_NODE" "$CLIENT_NODE" "$REGISTRY_NODE"; do
    is_forbidden_node "$endpoint" && die "forbidden live node selected: $endpoint"
  done
  for endpoint in "$worker_private_ip" "$BACKEND_PRIVATE_IP" "$minio_host" "$mongo_host" "$registry_host"; do
    is_forbidden_ip "$endpoint" && die "forbidden live private IP selected: $endpoint"
  done
  [[ $minio_host == "$BACKEND_PRIVATE_IP" ]] || die "MinIO must bind the isolated backend IP"
  [[ $mongo_host == "$BACKEND_PRIVATE_IP" ]] || die "MongoDB must bind the isolated backend IP"
  [[ $registry_host == "$BACKEND_PRIVATE_IP" ]] || die "registry must bind the isolated backend IP"
  [[ $dns_nameservers == "$BACKEND_PRIVATE_IP" ]] || die "guest DNS must use the isolated backend IP"

  [[ $backend_mount == /mnt/snapshare-zstd-streaming ]] \
    || die "backend mount must remain isolated: $backend_mount"
  [[ $minio_data_dir == "$backend_mount"/* ]] || die "MinIO data dir escaped the isolated mount"
  [[ $registry_data_dir == "$backend_mount"/* ]] || die "registry data dir escaped the isolated mount"
  [[ $cpu_state_dir == /users/Liquidz/cpu-baseline-zstd-* ]] \
    || die "CPU state dir lacks the zstd namespace: $cpu_state_dir"

  require_safe_name SNAPSHARE_MINIO_CONTAINER "$minio_container"
  require_safe_name SNAPSHARE_MONGODB_CONTAINER "$mongo_container"
  require_safe_name SNAPSHARE_REGISTRY_CONTAINER "$registry_container"
  require_safe_name SNAPSHARE_MONGODB_DB_VOLUME "$mongo_db_volume"
  require_safe_name SNAPSHARE_MONGODB_CONFIG_VOLUME "$mongo_config_volume"
  require_safe_name SNAPSHARE_DNS_TMUX_SESSION "$dns_tmux_session"
  require_safe_name REMOTE_BOOTSTRAP_DIR "$remote_bootstrap_dir"
  require_safe_name REMOTE_VHIVE_DIR "$remote_vhive_dir"
  require_safe_name REMOTE_RUNTIME_DIR "$remote_runtime_dir"
  require_safe_name REMOTE_VSWARM_DIR "$remote_vswarm_dir"
  [[ $minio_container == snapshare-zstd-* ]] || die "MinIO container lacks the zstd namespace"
  [[ $mongo_container == snapshare-zstd-* ]] || die "MongoDB container lacks the zstd namespace"
  [[ $registry_container == snapshare-zstd-* ]] || die "registry container lacks the zstd namespace"
  [[ $mongo_db_volume == snapshare-zstd-* ]] || die "MongoDB DB volume lacks the zstd namespace"
  [[ $mongo_config_volume == snapshare-zstd-* ]] || die "MongoDB config volume lacks the zstd namespace"
  [[ $dns_tmux_session == *zstd* ]] || die "DNS tmux session lacks the zstd namespace"
  [[ $remote_bootstrap_dir == *zstd* ]] || die "bootstrap dir lacks the zstd namespace"
  [[ $remote_vhive_dir == *zstd* ]] || die "vHive dir lacks the zstd namespace"
  [[ $remote_runtime_dir == *zstd* ]] || die "runtime dir lacks the zstd namespace"
  [[ $remote_vswarm_dir == *zstd* ]] || die "vSwarm dir lacks the zstd namespace"
  require_port SNAPSHARE_MINIO_PORT "$minio_port"
  require_port SNAPSHARE_MONGODB_PORT "$mongo_port"
  require_port SNAPSHARE_REGISTRY_PORT "$registry_port"
  require_port SNAPSHARE_DNS_PORT "$dns_port"
  require_port SNAPSHARE_PROVENANCE_HTTP_PORT "$provenance_http_port"
  require_port SNAPSHARE_RELAY_PORT "$relay_port"

  [[ $expected_relay_sha =~ ^[0-9a-f]{64}$ ]] \
    || die "set EXPECTED_RELAY_SHA to the frozen Zstd relay SHA-256"
  [[ $expected_vswarm_relay_sha =~ ^[0-9a-f]{64}$ ]] || die "invalid EXPECTED_VSWARM_RELAY_SHA"
  [[ $expected_guest_rootfs_sha =~ ^[0-9a-f]{64}$ ]] || die "invalid EXPECTED_GUEST_ROOTFS_SHA"
}

verify_local_assets() {
  require_file "$nodes_env"
  require_file "$service_archive"
  require_file "$mongo_dump"
  require_file "$provenance_archive"
  require_file "$function_archive"
  require_file "$relay_binary"
  require_file "$vswarm_relay_binary"
  require_file "$guest_rootfs"
  require_file "$runtime_asset_dir/bin/firecracker"
  require_file "$runtime_asset_dir/bin/jailer"
  require_file "$runtime_asset_dir/bin/containerd-shim-aws-firecracker"
  require_file "$runtime_asset_dir/bin/firecracker-containerd"
  require_file "$runtime_asset_dir/bin/firecracker-ctr"
  require_file "$runtime_asset_dir/bin/demux-snapshotter"
  require_file "$runtime_asset_dir/bin/http-address-resolver"
  require_file "$runtime_asset_dir/bin/grpcurl"
  require_file "$runtime_asset_dir/bin/vmlinux-5.10.186"
  require_file "$vhive_dir/configs/firecracker-containerd/config.toml"
  require_file "$vhive_dir/configs/firecracker-containerd/firecracker-runtime.json"
  require_file "$vhive_dir/configs/demux-snapshotter/config.toml"
  require_file "$vhive_dir/cmd/relay/image_map.json"
  require_file "$vhive_dir/scripts/create_devmapper.sh"
  require_file "$workspace_dir/vSwarm/utils/protobuf/helloworld/helloworld.proto"
  require_file "$script_dir/configure_c6620_cpu_baseline.sh"
  require_file "$script_dir/restart_snapshare_xl170_worker.sh"
  [[ $(sha256sum "$service_archive" | awk '{print $1}') == 3645eda742e65179856c49be0f0bf76636dcb846802c25738f7b7f0892fed5fc ]]
  [[ $(sha256sum "$mongo_dump" | awk '{print $1}') == b5b79b38d6dc9883c8a1c391edc1a3e3e008871a441c15f7ffb20e883fbe09e1 ]]
  [[ $(sha256sum "$provenance_archive" | awk '{print $1}') == c1c19dc2d3135742714ef1afe53ad0057c2449f9c5e40be9f2565e70a8705ba5 ]]
  [[ $(sha256sum "$function_archive" | awk '{print $1}') == 8fb2a0407473494bb07facd9fb5a29e075b38b7e7189f814076ee45d7551558a ]]
  [[ $(sha256sum "$relay_binary" | awk '{print $1}') == "$expected_relay_sha" ]] \
    || die "Zstd relay SHA does not match EXPECTED_RELAY_SHA"
  [[ $(sha256sum "$vswarm_relay_binary" | awk '{print $1}') == "$expected_vswarm_relay_sha" ]]
  [[ $(sha256sum "$guest_rootfs" | awk '{print $1}') == "$expected_guest_rootfs_sha" ]]
}

verify_nodes_read_only() {
  local forbidden_csv
  forbidden_csv=$(IFS=,; printf '%s' "${FORBIDDEN_PRIVATE_IPS[*]}")
  [[ -n $forbidden_csv ]] || forbidden_csv=-

  ssh "${ssh_opts[@]}" "$worker" bash -s -- \
    "$CONTROL_HOSTNAME" "$worker_private_ip" "$BACKEND_PRIVATE_IP" "$forbidden_csv" <<'REMOTE'
set -euo pipefail
expected_fqdn=$1
expected_ip=$2
peer=$3
forbidden_csv=$4
test "$(hostname -f)" = "$expected_fqdn"
hostname -I | tr ' ' '\n' | grep -Fx "$expected_ip"
test -c /dev/kvm
test "$(lscpu -J | jq -r '.lscpu[] | select(.field == "Model name:") | .data')" = "INTEL(R) XEON(R) GOLD 5512U"
ip -o route get "$peer" | grep -q "src $expected_ip"
IFS=, read -ra forbidden_ips <<<"$forbidden_csv"
for forbidden in "${forbidden_ips[@]}"; do
  [[ $forbidden == - ]] && continue
  ! hostname -I | tr ' ' '\n' | grep -Fxq "$forbidden"
done
if grep -q 'docker-registry.registry.svc.cluster.local' /etc/hosts; then
  registry_ip=$(awk '$2 == "docker-registry.registry.svc.cluster.local" {print $1; exit}' /etc/hosts)
  test -z "$registry_ip" || test "$registry_ip" = "$peer"
fi
REMOTE

  ssh "${ssh_opts[@]}" "$BACKEND_NODE" bash -s -- \
    "$BACKEND_HOSTNAME" "$BACKEND_PRIVATE_IP" "$worker_private_ip" "$forbidden_csv" \
    "$backend_device" "$backend_device_serial" "$backend_mount" <<'REMOTE'
set -euo pipefail
expected_fqdn=$1
expected_ip=$2
peer=$3
forbidden_csv=$4
device=$5
expected_serial=$6
mountpoint=$7
test "$(hostname -f)" = "$expected_fqdn"
hostname -I | tr ' ' '\n' | grep -Fx "$expected_ip"
test -c /dev/kvm
test "$(lscpu -J | jq -r '.lscpu[] | select(.field == "Model name:") | .data')" = "INTEL(R) XEON(R) GOLD 5512U"
ip -o route get "$peer" | grep -q "src $expected_ip"
IFS=, read -ra forbidden_ips <<<"$forbidden_csv"
for forbidden in "${forbidden_ips[@]}"; do
  [[ $forbidden == - ]] && continue
  ! hostname -I | tr ' ' '\n' | grep -Fxq "$forbidden"
done
test -b "$device"
observed_serial=$(lsblk -dnro SERIAL "$device" | xargs)
test "$observed_serial" = "$expected_serial"
if findmnt -rn -S "$device" >/dev/null 2>&1; then
  test "$(findmnt -rn -S "$device" -o TARGET)" = "$mountpoint"
else
  test -z "$(lsblk -dnro MOUNTPOINTS "$device" | tr -d '[:space:]')"
fi
REMOTE

  if [[ $loader != "$worker" && $loader != "$BACKEND_NODE" ]]; then
    ssh "${ssh_opts[@]}" "$loader" bash -s -- \
      "$LOADER_HOSTNAME" "$loader_private_ip" "$worker_private_ip" "$forbidden_csv" <<'REMOTE'
set -euo pipefail
expected_fqdn=$1
expected_ip=$2
peer=$3
forbidden_csv=$4
test "$(hostname -f)" = "$expected_fqdn"
hostname -I | tr ' ' '\n' | grep -Fx "$expected_ip"
test -c /dev/kvm
test "$(lscpu -J | jq -r '.lscpu[] | select(.field == "Model name:") | .data')" = "INTEL(R) XEON(R) GOLD 5512U"
ip -o route get "$peer" | grep -q "src $expected_ip"
IFS=, read -ra forbidden_ips <<<"$forbidden_csv"
for forbidden in "${forbidden_ips[@]}"; do
  [[ $forbidden == - ]] && continue
  ! hostname -I | tr ' ' '\n' | grep -Fxq "$forbidden"
done
REMOTE
  fi
}

install_packages() {
  log "install required packages on the isolated worker and backend"
  ssh "${ssh_opts[@]}" "$worker" 'bash -s' <<'REMOTE' &
set -euo pipefail
export DEBIAN_FRONTEND=noninteractive
sudo apt-get update
sudo apt-get install -y --no-install-recommends \
  bc ca-certificates containernetworking-plugins curl jq nftables iproute2 \
  iptables python3-boto3 rsync thin-provisioning-tools tmux zstd
REMOTE
  worker_pid=$!
  ssh "${ssh_opts[@]}" "$BACKEND_NODE" 'bash -s' <<'REMOTE' &
set -euo pipefail
export DEBIAN_FRONTEND=noninteractive
sudo apt-get update
sudo apt-get install -y --no-install-recommends \
  ca-certificates curl dnsmasq-base docker.io iproute2 jq rsync skopeo tmux \
  unzip xfsprogs zstd
sudo systemctl enable --now docker
REMOTE
  backend_pid=$!
  loader_pid=
  if [[ $loader != "$worker" && $loader != "$BACKEND_NODE" ]]; then
    ssh "${ssh_opts[@]}" "$loader" 'bash -s' <<'REMOTE' &
set -euo pipefail
export DEBIAN_FRONTEND=noninteractive
sudo apt-get update
sudo apt-get install -y --no-install-recommends \
  ca-certificates curl iproute2 jq rsync tmux zstd
REMOTE
    loader_pid=$!
  fi
  wait "$worker_pid"
  wait "$backend_pid"
  [[ -z $loader_pid ]] || wait "$loader_pid"
}

stage_archives() {
  log "stage durable assets into the zstd-only namespace"
  ssh "${ssh_opts[@]}" "$worker" bash -s -- \
    "$remote_bootstrap_dir" "$remote_vhive_dir" "$remote_vswarm_dir" "$remote_runtime_dir" <<'REMOTE'
set -euo pipefail
mkdir -p "$HOME/$1" "$HOME/$2/bin" "$HOME/$2/cmd/relay" \
  "$HOME/$2/configs" "$HOME/$2/scripts" \
  "$HOME/$3/tools/relay" "$HOME/$3/utils/protobuf/helloworld" "$HOME/$4"
REMOTE
  ssh "${ssh_opts[@]}" "$BACKEND_NODE" bash -s -- "$remote_bootstrap_dir" <<'REMOTE'
set -euo pipefail
mkdir -p "$HOME/$1"
REMOTE

  rsync -a --partial --info=progress2 -e "$rsync_ssh" \
    "$provenance_archive" "$BACKEND_NODE:~/$remote_bootstrap_dir/function-provenance-images.tar.zst" &
  provenance_pid=$!
  rsync -a --partial --info=progress2 -e "$rsync_ssh" \
    "$service_archive" "$mongo_dump" "$function_archive" \
    "$BACKEND_NODE:~/$remote_bootstrap_dir/" &
  backend_assets_pid=$!

  rsync -a --partial -e "$rsync_ssh" \
    "$runtime_asset_dir/bin/firecracker" \
    "$runtime_asset_dir/bin/jailer" \
    "$runtime_asset_dir/bin/containerd-shim-aws-firecracker" \
    "$runtime_asset_dir/bin/firecracker-containerd" \
    "$runtime_asset_dir/bin/firecracker-ctr" \
    "$runtime_asset_dir/bin/demux-snapshotter" \
    "$runtime_asset_dir/bin/http-address-resolver" \
    "$runtime_asset_dir/bin/grpcurl" \
    "$guest_rootfs" \
    "$runtime_asset_dir/bin/vmlinux-5.10.186" \
    "$relay_binary" \
    "$worker:~/$remote_vhive_dir/bin/"
  ssh "${ssh_opts[@]}" "$worker" bash -s -- \
    "$remote_vhive_dir" "$(basename -- "$relay_binary")" <<'REMOTE'
set -euo pipefail
vhive=$HOME/$1
relay_name=$2
mv -f "$vhive/bin/default-rootfs.plain-http-registry-20260826.img" "$vhive/bin/default-rootfs.img"
if [[ $relay_name != relay ]]; then
  cp -f "$vhive/bin/$relay_name" "$vhive/bin/relay"
fi
REMOTE
  rsync -a --partial -e "$rsync_ssh" \
    "$vhive_dir/configs/firecracker-containerd" \
    "$vhive_dir/configs/demux-snapshotter" \
    "$worker:~/$remote_vhive_dir/configs/"
  rsync -a --partial -e "$rsync_ssh" \
    "$vhive_dir/cmd/relay/image_map.json" \
    "$worker:~/$remote_vhive_dir/cmd/relay/image_map.json"
  rsync -a --partial -e "$rsync_ssh" \
    "$vhive_dir/scripts/create_devmapper.sh" \
    "$worker:~/$remote_vhive_dir/scripts/create_devmapper.sh"
  rsync -a --partial -e "$rsync_ssh" \
    "$script_dir/configure_c6620_cpu_baseline.sh" \
    "$worker:~/$remote_bootstrap_dir/"
  rsync -a --partial -e "$rsync_ssh" \
    "$script_dir/restart_snapshare_xl170_worker.sh" \
    "$worker:~/restart_snapshare_zstd_streaming_worker.sh"
  rsync -a --partial -e "$rsync_ssh" \
    "$script_dir/configure_c6620_cpu_baseline.sh" \
    "$BACKEND_NODE:~/$remote_bootstrap_dir/"
  rsync -a --partial -e "$rsync_ssh" \
    "$vswarm_relay_binary" "$worker:~/$remote_vswarm_dir/tools/relay/server"
  rsync -a --partial -e "$rsync_ssh" \
    "$workspace_dir/vSwarm/utils/protobuf/helloworld/" \
    "$worker:~/$remote_vswarm_dir/utils/protobuf/helloworld/"

  wait "$provenance_pid"
  wait "$backend_assets_pid"

  log "copy the provenance archive from $BACKEND_NODE to $worker over the private link"
  ssh "${ssh_opts[@]}" "$BACKEND_NODE" bash -s -- \
    "$remote_bootstrap_dir" "$BACKEND_PRIVATE_IP" "$provenance_http_port" <<'REMOTE'
set -euo pipefail
bootstrap_dir=$1
private_ip=$2
port=$3
pid_file=$HOME/$bootstrap_dir/provenance-http.pid
test ! -e "$pid_file"
nohup python3 -m http.server --bind "$private_ip" "$port" \
  --directory "$HOME/$bootstrap_dir" \
  >"$HOME/$bootstrap_dir/provenance-http.log" 2>&1 &
echo $! >"$pid_file"
REMOTE
  if ! ssh "${ssh_opts[@]}" "$worker" bash -s -- \
    "$remote_bootstrap_dir" "$BACKEND_PRIVATE_IP" "$provenance_http_port" <<'REMOTE'
set -euo pipefail
bootstrap_dir=$1
backend_ip=$2
port=$3
destination=$HOME/$bootstrap_dir/function-provenance-images.tar.zst
curl -fL --retry 5 --output "$destination.new" \
  "http://$backend_ip:$port/function-provenance-images.tar.zst"
printf '%s  %s\n' \
  c1c19dc2d3135742714ef1afe53ad0057c2449f9c5e40be9f2565e70a8705ba5 \
  "$destination.new" | sha256sum -c -
mv -f "$destination.new" "$destination"
REMOTE
  then
    ssh "${ssh_opts[@]}" "$BACKEND_NODE" bash -s -- "$remote_bootstrap_dir" <<'REMOTE'
set -euo pipefail
pid_file=$HOME/$1/provenance-http.pid
test ! -s "$pid_file" || kill "$(cat "$pid_file")"
rm -f "$pid_file"
REMOTE
    return 1
  fi
  ssh "${ssh_opts[@]}" "$BACKEND_NODE" bash -s -- "$remote_bootstrap_dir" <<'REMOTE'
set -euo pipefail
pid_file=$HOME/$1/provenance-http.pid
kill "$(cat "$pid_file")"
rm -f "$pid_file"
REMOTE
}

prepare_backend_disk() {
  log "prepare guarded zstd backend disk $backend_device"
  ssh "${ssh_opts[@]}" "$BACKEND_NODE" bash -s -- \
    "$backend_device" "$backend_device_serial" "$backend_mount" \
    "$minio_data_dir" "$registry_data_dir" "${ALLOW_FORMAT_ZSTD_BACKEND:-0}" <<'REMOTE'
set -euo pipefail
device=$1
expected_serial=$2
mountpoint=$3
minio_data_dir=$4
registry_data_dir=$5
allow_format=$6
test -b "$device"
test "$(lsblk -dnro SERIAL "$device" | xargs)" = "$expected_serial"
if findmnt -rn "$mountpoint" >/dev/null 2>&1; then
  test "$(findmnt -rn -o SOURCE "$mountpoint")" = "$device"
else
  test "$(lsblk -nrpo NAME,TYPE "$device" | wc -l)" -eq 1
  test "$(lsblk -nrpo TYPE "$device")" = disk
  test -z "$(lsblk -nrpo MOUNTPOINTS "$device" | tr -d '[:space:]')"
  fstype=$(lsblk -dnro FSTYPE "$device")
  if [[ -z $fstype ]]; then
    test "$allow_format" = 1 || {
      echo "refusing to format without ALLOW_FORMAT_ZSTD_BACKEND=1" >&2
      exit 10
    }
    test -z "$(sudo wipefs -n "$device")"
    sudo mkfs.xfs -f "$device"
  else
    test "$fstype" = xfs
  fi
  sudo mkdir -p "$mountpoint"
  uuid=$(sudo blkid -s UUID -o value "$device")
  grep -qF "UUID=$uuid $mountpoint xfs" /etc/fstab || \
    printf 'UUID=%s %s xfs defaults,noatime 0 2\n' "$uuid" "$mountpoint" | sudo tee -a /etc/fstab >/dev/null
  sudo mount "$mountpoint"
fi
sudo install -d -o 1000 -g 1000 "$minio_data_dir" "$registry_data_dir"
findmnt "$mountpoint"
REMOTE
}

start_backend_services() {
  log "start isolated MinIO, MongoDB, registry, and DNS on $BACKEND_NODE"
  ssh "${ssh_opts[@]}" "$BACKEND_NODE" bash -s -- \
    "$BACKEND_PRIVATE_IP" "$remote_bootstrap_dir" \
    "$minio_container" "$mongo_container" "$registry_container" \
    "$mongo_db_volume" "$mongo_config_volume" \
    "$minio_data_dir" "$registry_data_dir" \
    "$minio_port" "$mongo_port" "$registry_port" \
    "$dns_port" "$dns_tmux_session" <<'REMOTE'
set -euo pipefail
private_ip=$1
bootstrap_dir=$2
minio_container=$3
mongo_container=$4
registry_container=$5
mongo_db_volume=$6
mongo_config_volume=$7
minio_data_dir=$8
registry_data_dir=$9
shift 9
minio_port=$1
mongo_port=$2
registry_port=$3
dns_port=$4
dns_tmux_session=$5
archive=$HOME/$bootstrap_dir/docker-service-images.tar.zst
mongo_dump=$HOME/$bootstrap_dir/snapshare-mongodb.archive.gz

if ! sudo docker image inspect quay.io/minio/minio:RELEASE.2024-12-18T13-15-44Z >/dev/null 2>&1 \
  || ! sudo docker image inspect vhiveease/mongodb:latest >/dev/null 2>&1; then
  zstd -dc "$archive" | sudo docker load
fi
sudo docker image inspect quay.io/minio/minio:RELEASE.2024-12-18T13-15-44Z >/dev/null
if ! sudo docker image inspect vhiveease/mongodb:latest >/dev/null 2>&1; then
  sudo docker tag sha256:6223e6e877fe91c08b2fb87fb1135d05d9c704d1cd423191fbd687465b35a106 vhiveease/mongodb:latest
fi
sudo docker image inspect registry:2.8.3 >/dev/null 2>&1 || sudo docker pull registry:2.8.3

if ! sudo docker container inspect "$registry_container" >/dev/null 2>&1; then
  sudo docker run -d --name "$registry_container" --restart unless-stopped \
    -p "$private_ip:$registry_port:5000" -v "$registry_data_dir:/var/lib/registry" registry:2.8.3 >/dev/null
else
  sudo docker start "$registry_container" >/dev/null
fi
if ! sudo docker container inspect "$minio_container" >/dev/null 2>&1; then
  sudo docker run -d --name "$minio_container" --restart unless-stopped \
    -p "$private_ip:$minio_port:9000" \
    -e MINIO_ROOT_USER=minio -e MINIO_ROOT_PASSWORD=minio123 \
    -v "$minio_data_dir:/data" \
    quay.io/minio/minio:RELEASE.2024-12-18T13-15-44Z server /data >/dev/null
else
  sudo docker start "$minio_container" >/dev/null
fi
sudo docker volume inspect "$mongo_db_volume" >/dev/null 2>&1 || sudo docker volume create "$mongo_db_volume" >/dev/null
sudo docker volume inspect "$mongo_config_volume" >/dev/null 2>&1 || sudo docker volume create "$mongo_config_volume" >/dev/null
if ! sudo docker container inspect "$mongo_container" >/dev/null 2>&1; then
  test "$mongo_port" = 27017
  sudo docker run -d --name "$mongo_container" --restart unless-stopped --network host \
    -v "$mongo_db_volume:/data/db" -v "$mongo_config_volume:/data/configdb" \
    vhiveease/mongodb:latest --bind_ip "127.0.0.1,$private_ip" >/dev/null
else
  sudo docker start "$mongo_container" >/dev/null
fi

for _ in $(seq 1 90); do
  curl -fsS "http://$private_ip:$minio_port/minio/health/ready" >/dev/null 2>&1 \
    && timeout 1 bash -c "</dev/tcp/$private_ip/$mongo_port" 2>/dev/null \
    && curl -fsS "http://$private_ip:$registry_port/v2/" >/dev/null 2>&1 && break
  sleep 1
done
curl -fsS "http://$private_ip:$minio_port/minio/health/ready" >/dev/null
timeout 1 bash -c "</dev/tcp/$private_ip/$mongo_port"
curl -fsS "http://$private_ip:$registry_port/v2/" >/dev/null

if ! sudo docker exec "$mongo_container" mongo --quiet --eval \
  'db.getMongo().getDBNames().indexOf("image_db") >= 0 ? quit(0) : quit(1)' >/dev/null 2>&1; then
  sudo docker exec -i "$mongo_container" mongorestore --archive --gzip <"$mongo_dump"
fi

if ! tmux has-session -t "$dns_tmux_session" 2>/dev/null; then
  tmux new-session -d -s "$dns_tmux_session" \
    "sudo dnsmasq --no-daemon --keep-in-foreground --bind-interfaces --listen-address=$private_ip --port=$dns_port --address=/docker-registry.registry.svc.cluster.local/$private_ip --server=8.8.8.8 --log-queries --log-facility=- > '$HOME/snapshare-zstd-dnsmasq.log' 2>&1"
fi
REMOTE

  log "push the pinned image archive into the isolated backend registry"
  ssh "${ssh_opts[@]}" "$BACKEND_NODE" bash -s -- \
    "$registry_host" "$registry_port" "$remote_bootstrap_dir" "$function_tag" <<'REMOTE'
set -euo pipefail
registry_host=$1
registry_port=$2
bootstrap_dir=$3
tag=$4
archive=$HOME/$bootstrap_dir/image-rotate-go.restore-reconnect-f268d11-r2-20260818.esgz.oci.tar
target="docker://$registry_host:$registry_port/liquidzk/image-rotate-go:$tag"
skopeo copy --dest-tls-verify=false "oci-archive:$archive" "$target"
skopeo inspect --tls-verify=false "$target" | jq -r '.Digest, .Name, .Tag'
REMOTE
}

install_worker_runtime() {
  log "install the isolated Zstd relay and Firecracker assets on $worker"
  ssh "${ssh_opts[@]}" "$worker" bash -s -- \
    "$BACKEND_PRIVATE_IP" "$remote_bootstrap_dir" "$remote_vhive_dir" "$remote_vswarm_dir" \
    "$expected_relay_sha" "$expected_guest_rootfs_sha" <<'REMOTE'
set -euo pipefail
backend_ip=$1
bootstrap_dir=$2
remote_vhive_dir=$3
remote_vswarm_dir=$4
expected_relay_sha=$5
expected_guest_rootfs_sha=$6
vhive=$HOME/$remote_vhive_dir
sudo mkdir -p /usr/local/bin /var/lib/firecracker-containerd/runtime \
  /var/lib/demux-snapshotter /etc/firecracker-containerd \
  /etc/demux-snapshotter /etc/containerd /opt/cni/bin /users/Liquidz
for binary in firecracker jailer containerd-shim-aws-firecracker \
  firecracker-containerd firecracker-ctr demux-snapshotter http-address-resolver; do
  sudo install -m 0755 "$vhive/bin/$binary" "/usr/local/bin/$binary"
done
sudo install -m 0644 "$vhive/bin/default-rootfs.img" /var/lib/firecracker-containerd/runtime/default-rootfs.img
sudo install -m 0644 "$vhive/bin/vmlinux-5.10.186" /var/lib/firecracker-containerd/runtime/hello-vmlinux.bin
sudo install -m 0644 "$vhive/configs/firecracker-containerd/config.toml" /etc/firecracker-containerd/config.toml
sudo install -m 0644 "$vhive/configs/firecracker-containerd/firecracker-runtime.json" /etc/containerd/firecracker-runtime.json
sudo install -m 0644 "$vhive/configs/demux-snapshotter/config.toml" /etc/demux-snapshotter/config.toml

cni_source=
for candidate in /usr/lib/cni /usr/libexec/cni; do
  if compgen -G "$candidate/*" >/dev/null; then cni_source=$candidate; break; fi
done
test -n "$cni_source"
sudo install -m 0755 "$cni_source"/* /opt/cni/bin/

if [[ ! -f /users/Liquidz/images/rootfs.tar ]]; then
  sudo tar --zstd -xf "$HOME/$bootstrap_dir/function-provenance-images.tar.zst" -C /users/Liquidz
  sudo chown -R "$(id -u):$(id -g)" /users/Liquidz/images
fi
sudo sysctl -w net.ipv4.ip_forward=1 >/dev/null
sudo modprobe kvm_intel
sudo modprobe dm_thin_pool

public_iface=$(ip -o route show default | awk 'NR == 1 {print $5}')
sudo nft list table ip nat >/dev/null 2>&1 || sudo nft 'add table ip nat'
sudo nft list chain ip nat SNAPSHARE_ZSTD_EGRESS >/dev/null 2>&1 || \
  sudo nft 'add chain ip nat SNAPSHARE_ZSTD_EGRESS { type nat hook postrouting priority srcnat; policy accept; }'
sudo nft flush chain ip nat SNAPSHARE_ZSTD_EGRESS
sudo nft add rule ip nat SNAPSHARE_ZSTD_EGRESS ip saddr 172.16.0.0/12 oifname "$public_iface" counter masquerade

sudo sed -i '/[[:space:]]docker-registry\.registry\.svc\.cluster\.local$/d' /etc/hosts
printf '%s %s\n' "$backend_ip" docker-registry.registry.svc.cluster.local | sudo tee -a /etc/hosts >/dev/null

# Keep the old generic call helper compatible without sharing artifacts with
# another node or worktree: both canonical names resolve to zstd-only trees.
ln -sfn "$HOME/$remote_vhive_dir" "$HOME/vhive-snapshare"
ln -sfn "$HOME/$remote_vswarm_dir" "$HOME/vswarm"

test "$(sha256sum "$vhive/bin/relay" | awk '{print $1}')" = "$expected_relay_sha"
test "$(sha256sum /var/lib/firecracker-containerd/runtime/default-rootfs.img | awk '{print $1}')" = "$expected_guest_rootfs_sha"
test "$(getent hosts docker-registry.registry.svc.cluster.local | awk '{print $1; exit}')" = "$backend_ip"
test -c /dev/kvm
test -f /users/Liquidz/images/rootfs.tar
chmod 0755 "$HOME/restart_snapshare_zstd_streaming_worker.sh"
REMOTE

  log "create or validate the Zstd worker's dedicated-host devmapper pool"
  ssh "${ssh_opts[@]}" "$worker" bash -s -- "$remote_vhive_dir" <<'REMOTE'
set -euo pipefail
vhive=$HOME/$1
sudo dmsetup info fc-dev-thinpool >/dev/null 2>&1 || "$vhive/scripts/create_devmapper.sh"
sudo dmsetup info fc-dev-thinpool
REMOTE
}

apply_cpu_baseline() {
  log "apply the independent 2.1-GHz, SMT-off, Turbo-off CPU baseline"
  local -A seen=()
  for host in "$worker" "$BACKEND_NODE" "$loader"; do
    [[ -z ${seen[$host]:-} ]] || continue
    seen[$host]=1
    if ssh "${ssh_opts[@]}" "$host" "test -f '$cpu_state_dir/apply.complete'"; then
      ssh "${ssh_opts[@]}" "$host" 'bash -s -- verify-2.1ghz' <"$script_dir/configure_c6620_cpu_baseline.sh"
    else
      ssh "${ssh_opts[@]}" "$host" "sudo bash -s -- apply-2.1ghz '$cpu_state_dir'" <"$script_dir/configure_c6620_cpu_baseline.sh"
    fi
  done
}

apply_bandwidth_limit() {
  local mode=${SNAPSHARE_BANDWIDTH_MODE:-backend-egress}
  if [[ $mode == backend-egress ]]; then
    log "apply the 10-Gbit/s backend-to-worker limit on the isolated backend only"
    ssh "${ssh_opts[@]}" "$BACKEND_NODE" bash -s -- "$worker_private_ip" <<'REMOTE'
set -euo pipefail
peer=$1
iface=$(ip -o route get "$peer" | awk '{for (i=1; i<=NF; i++) if ($i == "dev") {print $(i+1); exit}}')
test -n "$iface"
sudo tc qdisc replace dev "$iface" root tbf rate 10gbit burst 2mb latency 50ms
tc qdisc show dev "$iface"
REMOTE
  elif [[ $mode == worker-ingress-ifb-legacy ]]; then
    log "apply historical 10-Gbit/s worker-ingress IFB limit"
    ssh "${ssh_opts[@]}" "$worker" bash -s -- "$BACKEND_PRIVATE_IP" <<'REMOTE'
set -euo pipefail
peer=$1
iface=$(ip -o route get "$peer" | awk '{for (i=1; i<=NF; i++) if ($i == "dev") {print $(i+1); exit}}')
test -n "$iface"
sudo modprobe ifb numifbs=1
sudo ip link set dev ifb0 up
sudo tc qdisc del dev "$iface" ingress 2>/dev/null || true
sudo tc qdisc add dev "$iface" handle ffff: ingress
sudo tc filter add dev "$iface" parent ffff: protocol all u32 match u32 0 0 \
  action mirred egress redirect dev ifb0
sudo tc qdisc replace dev ifb0 root tbf rate 10gbit burst 32mbit latency 400ms
tc qdisc show dev "$iface"
tc filter show dev "$iface" parent ffff:
tc qdisc show dev ifb0
REMOTE
    ssh "${ssh_opts[@]}" "$BACKEND_NODE" bash -s -- "$worker_private_ip" <<'REMOTE'
set -euo pipefail
peer=$1
iface=$(ip -o route get "$peer" | awk '{for (i=1; i<=NF; i++) if ($i == "dev") {print $(i+1); exit}}')
! tc qdisc show dev "$iface" | grep -q '^qdisc tbf '
REMOTE
  else
    die "unknown SNAPSHARE_BANDWIDTH_MODE: $mode"
  fi
}

validate_platform() {
  log "validate that every live endpoint belongs to the authorized Zstd pair"
  ssh "${ssh_opts[@]}" "$BACKEND_NODE" bash -s -- \
    "$BACKEND_HOSTNAME" "$BACKEND_PRIVATE_IP" "$backend_mount" \
    "$minio_container" "$mongo_container" "$registry_container" \
    "$minio_port" "$mongo_port" "$registry_port" "$function_tag" <<'REMOTE'
set -euo pipefail
expected_fqdn=$1
private_ip=$2
mountpoint=$3
minio_container=$4
mongo_container=$5
registry_container=$6
minio_port=$7
mongo_port=$8
registry_port=$9
shift 9
tag=$1
test "$(hostname -f)" = "$expected_fqdn"
test "$(sudo docker inspect -f '{{.State.Running}}' "$minio_container")" = true
test "$(sudo docker inspect -f '{{.State.Running}}' "$mongo_container")" = true
test "$(sudo docker inspect -f '{{.State.Running}}' "$registry_container")" = true
curl -fsS "http://$private_ip:$minio_port/minio/health/ready" >/dev/null
timeout 3 bash -c "</dev/tcp/$private_ip/$mongo_port"
curl -fsS "http://$private_ip:$registry_port/v2/liquidzk/image-rotate-go/manifests/$tag" \
  -H 'Accept: application/vnd.oci.image.manifest.v1+json' >/dev/null
findmnt "$mountpoint"
REMOTE
  ssh "${ssh_opts[@]}" "$worker" bash -s -- \
    "$CONTROL_HOSTNAME" "$BACKEND_PRIVATE_IP" "$minio_port" "$mongo_port" "$registry_port" \
    "$expected_relay_sha" "$remote_vhive_dir" <<'REMOTE'
set -euo pipefail
expected_fqdn=$1
backend_ip=$2
minio_port=$3
mongo_port=$4
registry_port=$5
expected_relay_sha=$6
remote_vhive_dir=$7
test "$(hostname -f)" = "$expected_fqdn"
timeout 3 bash -c "</dev/tcp/$backend_ip/$minio_port"
timeout 3 bash -c "</dev/tcp/$backend_ip/$mongo_port"
timeout 3 bash -c "</dev/tcp/$backend_ip/$registry_port"
test -c /dev/kvm
sudo dmsetup info fc-dev-thinpool >/dev/null
test "$(sha256sum "$HOME/$remote_vhive_dir/bin/relay" | awk '{print $1}')" = "$expected_relay_sha"
test "$(getent hosts docker-registry.registry.svc.cluster.local | awk '{print $1; exit}')" = "$backend_ip"
REMOTE
  if [[ $loader != "$worker" && $loader != "$BACKEND_NODE" ]]; then
    ssh "${ssh_opts[@]}" "$loader" bash -s -- \
      "$LOADER_HOSTNAME" "$loader_private_ip" "$worker_private_ip" "$BACKEND_PRIVATE_IP" \
      "$minio_port" "$mongo_port" "$registry_port" <<'REMOTE'
set -euo pipefail
expected_fqdn=$1
expected_ip=$2
worker_ip=$3
backend_ip=$4
minio_port=$5
mongo_port=$6
registry_port=$7
test "$(hostname -f)" = "$expected_fqdn"
hostname -I | tr ' ' '\n' | grep -Fx "$expected_ip"
timeout 3 bash -c "</dev/tcp/$worker_ip/22"
timeout 3 bash -c "</dev/tcp/$backend_ip/$minio_port"
timeout 3 bash -c "</dev/tcp/$backend_ip/$mongo_port"
timeout 3 bash -c "</dev/tcp/$backend_ip/$registry_port"
REMOTE
  fi
  log "ZSTD_C6620_PLATFORM_READY worker=$worker backend=$BACKEND_NODE result_family=${RESULT_FAMILY:-zstd-streaming}"
}

verify_environment_isolation
verify_local_assets
verify_nodes_read_only

if [[ ${PREFLIGHT_ONLY:-0} == 1 ]]; then
  log "ZSTD_C6620_PREFLIGHT_PASS worker=$worker backend=$BACKEND_NODE"
  exit 0
fi

[[ ${ALLOW_ZSTD_BOOTSTRAP:-} == "$EXPERIMENT_ID" ]] || die \
  "set ALLOW_ZSTD_BOOTSTRAP=$EXPERIMENT_ID to authorize writes to the selected Zstd pair"

install_packages
stage_archives
prepare_backend_disk
start_backend_services
install_worker_runtime
apply_cpu_baseline
apply_bandwidth_limit
validate_platform
