#!/usr/bin/env bash
# Restart the bounded SnapShare runtime on one evaluation worker. Run this
# script on the worker itself after the Firecracker and provenance assets are
# installed. The private interface is detected from the MinIO route so the same
# script works on both xl170 and c6620 CloudLab nodes.
set -euo pipefail

if (( $# < 2 || $# > 7 )); then
  echo "usage: $0 <ws-recording:0|1> <vm-mib> [log-label] [cache-size] [reuse-local-cache:0|1] [snapshot-mode:remote|local] [security-mode]" >&2
  exit 2
fi

recording=$1
vm_mib=$2
label=${3:-formal}
cache_size=${4:-15000}
reuse_local_cache=${5:-0}
snapshot_mode=${6:-remote}
security_mode=${7:-partial}
clean_local_snapshot_cache=${CLEAN_LOCAL_SNAPSHOT_CACHE:-0}
[[ $recording == 0 || $recording == 1 ]] || { echo "ws-recording must be 0 or 1" >&2; exit 2; }
[[ $vm_mib =~ ^[1-9][0-9]*$ ]] || { echo "vm-mib must be positive" >&2; exit 2; }
[[ $label =~ ^[a-zA-Z0-9._-]+$ ]] || { echo "invalid log label" >&2; exit 2; }
[[ $cache_size =~ ^[1-9][0-9]*$ ]] || { echo "cache-size must be positive" >&2; exit 2; }
[[ $reuse_local_cache == 0 || $reuse_local_cache == 1 ]] || { echo "reuse-local-cache must be 0 or 1" >&2; exit 2; }
[[ $clean_local_snapshot_cache == 0 || $clean_local_snapshot_cache == 1 ]] || { echo "CLEAN_LOCAL_SNAPSHOT_CACHE must be 0 or 1" >&2; exit 2; }
if [[ $reuse_local_cache == 1 && $clean_local_snapshot_cache == 1 ]]; then
  echo "cannot clean and reuse the local snapshot cache at the same time" >&2
  exit 2
fi
[[ $snapshot_mode == remote || $snapshot_mode == local ]] || { echo "snapshot-mode must be remote or local" >&2; exit 2; }
case $security_mode in
  none|full-dedup|partial|no-image-sharing|full) ;;
  *) echo "security-mode must be none, full-dedup, partial, no-image-sharing, or full" >&2; exit 2 ;;
esac

runtime_dir=${SNAPSHARE_RUNTIME_DIR:-$HOME/snapshare-runtime}
relay_dir=${SNAPSHARE_RELAY_DIR:-$HOME/vhive-snapshare/cmd/relay}
relay_binary=${SNAPSHARE_RELAY_BINARY:-$HOME/vhive-snapshare/bin/relay}
relay_endpoint=${SNAPSHARE_RELAY_ENDPOINT:-0.0.0.0}
rootfs=/var/lib/firecracker-containerd/runtime/default-rootfs.img
images_dir=${SNAPSHARE_IMAGES_DIR:-/users/Liquidz/images}
minio_host=${SNAPSHARE_MINIO_HOST:-10.0.1.2}
minio_port=${SNAPSHARE_MINIO_PORT:-9000}
threads=${SNAPSHARE_THREADS:-8}
enable_chunking=${SNAPSHARE_CHUNKING:-1}
chunk_size=${SNAPSHARE_CHUNK_SIZE:-4096}
enable_ws_coalescing=${SNAPSHARE_WS_COALESCING:-1}
enable_lazy=${SNAPSHARE_LAZY:-1}
enable_base_snap=${SNAPSHARE_BASE_SNAP:-0}
enable_ws_compression=${SNAPSHARE_WS_COMPRESSION:-0}
enable_chunk_compression=${SNAPSHARE_CHUNK_COMPRESSION:-0}
zstd_level=${SNAPSHARE_ZSTD_LEVEL:-3}
zstd_frame_size=${SNAPSHARE_ZSTD_FRAME_SIZE:-1048576}
zstd_fetchers=${SNAPSHARE_ZSTD_FETCHERS:-10}
net_pool_size=${SNAPSHARE_NET_POOL_SIZE:-10}
veth_prefix=${SNAPSHARE_VETH_PREFIX:-172.17}
clone_prefix=${SNAPSHARE_CLONE_PREFIX:-172.18}
dns_nameservers=${SNAPSHARE_DNS_NAMESERVERS:-}
clean_after_invocation=${SNAPSHARE_CLEAN_AFTER_INVOCATION:-0}
ready_timeout_seconds=${SNAPSHARE_READY_TIMEOUT_SECONDS:-120}
ws_profile_invocations=${SNAPSHARE_WS_PROFILE_INVOCATIONS:-0}
fresh_source_delay_seconds=${SNAPSHARE_FRESH_SOURCE_DELAY_SECONDS:-10}
[[ $minio_port =~ ^[1-9][0-9]*$ && $minio_port -le 65535 ]] || { echo "invalid SNAPSHARE_MINIO_PORT" >&2; exit 2; }
[[ $threads =~ ^[1-9][0-9]*$ ]] || { echo "invalid SNAPSHARE_THREADS" >&2; exit 2; }
[[ $chunk_size =~ ^[1-9][0-9]*$ && $((chunk_size % 4096)) -eq 0 ]] \
  || { echo "SNAPSHARE_CHUNK_SIZE must be positive and 4-KiB aligned" >&2; exit 2; }
[[ $net_pool_size =~ ^[1-9][0-9]*$ ]] || { echo "invalid SNAPSHARE_NET_POOL_SIZE" >&2; exit 2; }
[[ $veth_prefix =~ ^[0-9]{1,3}\.[0-9]{1,3}$ ]] || { echo "invalid SNAPSHARE_VETH_PREFIX" >&2; exit 2; }
[[ $clone_prefix =~ ^[0-9]{1,3}\.[0-9]{1,3}$ ]] || { echo "invalid SNAPSHARE_CLONE_PREFIX" >&2; exit 2; }
[[ $dns_nameservers =~ ^$|^[0-9a-fA-F:.,]+$ ]] || { echo "invalid SNAPSHARE_DNS_NAMESERVERS" >&2; exit 2; }
[[ $ready_timeout_seconds =~ ^[1-9][0-9]*$ ]] || { echo "invalid SNAPSHARE_READY_TIMEOUT_SECONDS" >&2; exit 2; }
[[ $ws_profile_invocations =~ ^[0-9]+$ ]] || { echo "invalid SNAPSHARE_WS_PROFILE_INVOCATIONS" >&2; exit 2; }
[[ $fresh_source_delay_seconds =~ ^[0-9]+$ ]] || { echo "invalid SNAPSHARE_FRESH_SOURCE_DELAY_SECONDS" >&2; exit 2; }
if (( ws_profile_invocations > 0 )); then
  [[ $recording == 1 ]] || { echo "SNAPSHARE_WS_PROFILE_INVOCATIONS requires ws-recording=1" >&2; exit 2; }
  [[ $clean_after_invocation == 0 ]] || { echo "profiling window is incompatible with per-invocation cleanup" >&2; exit 2; }
fi
[[ $relay_endpoint =~ ^([0-9]{1,3}\.){3}[0-9]{1,3}$ ]] \
  || { echo "SNAPSHARE_RELAY_ENDPOINT must be an IPv4 address" >&2; exit 2; }
for toggle in "$enable_chunking" "$enable_ws_coalescing" "$enable_lazy" "$enable_base_snap" \
  "$enable_ws_compression" "$enable_chunk_compression"; do
  [[ $toggle == 0 || $toggle == 1 ]] || { echo "SnapShare feature toggles must be 0 or 1" >&2; exit 2; }
done
[[ $zstd_level =~ ^-?[0-9]+$ ]] || { echo "invalid SNAPSHARE_ZSTD_LEVEL" >&2; exit 2; }
[[ $zstd_frame_size =~ ^[1-9][0-9]*$ && $((zstd_frame_size % 4096)) -eq 0 ]] \
  || { echo "SNAPSHARE_ZSTD_FRAME_SIZE must be positive and 4-KiB aligned" >&2; exit 2; }
[[ $zstd_fetchers =~ ^[1-9][0-9]*$ ]] || { echo "invalid SNAPSHARE_ZSTD_FETCHERS" >&2; exit 2; }
if [[ $enable_ws_compression == 1 && $enable_ws_coalescing == 0 ]]; then
  echo "SNAPSHARE_WS_COMPRESSION requires SNAPSHARE_WS_COALESCING=1" >&2
  exit 2
fi
if [[ $enable_chunk_compression == 1 && $enable_chunking == 0 ]]; then
  echo "SNAPSHARE_CHUNK_COMPRESSION requires SNAPSHARE_CHUNKING=1" >&2
  exit 2
fi
if [[ $security_mode == full-dedup && $enable_ws_coalescing == 1 ]]; then
  echo "full-dedup requires SNAPSHARE_WS_COALESCING=0 because coalesced private WS objects are revision-scoped" >&2
  exit 2
fi
[[ $clean_after_invocation == 0 || $clean_after_invocation == 1 ]] || { echo "SNAPSHARE_CLEAN_AFTER_INVOCATION must be 0 or 1" >&2; exit 2; }
host_iface=${SNAPSHARE_HOST_IFACE:-$(ip -o route get "$minio_host" | awk '{for (i=1; i<=NF; i++) if ($i == "dev") {print $(i+1); exit}}')}
[[ -n $host_iface ]] || { echo "could not determine private interface for $minio_host" >&2; exit 3; }

for path in "$relay_binary" "$rootfs" "$images_dir/rootfs.tar"; do
  [[ -e $path ]] || { echo "missing required runtime asset: $path" >&2; exit 3; }
done

mkdir -p "$runtime_dir"
# Give the relay's SIGTERM handler time to remove its network pool before the
# surrounding tmux session and the lower-level runtime are stopped. Killing the
# session first leaks uvmns/veth/nft objects; enough repeated restarts can hit
# the kernel's practical hooked-chain limit and silently break guest egress.
relay_pids=$(pgrep -f "^${relay_binary} " || true)
if [[ -n $relay_pids ]]; then
  sudo kill -TERM $relay_pids
  for _ in $(seq 1 60); do
    still_running=false
    for relay_pid in $relay_pids; do
      if sudo kill -0 "$relay_pid" 2>/dev/null; then
        still_running=true
        break
      fi
    done
    [[ $still_running == false ]] && break
    sleep 1
  done
fi
for session in relay_snapshare demux_snapshare resolver_snapshare fccd_snapshare; do
  tmux kill-session -t "$session" 2>/dev/null || true
done
sleep 2

patterns=(
  "^${relay_binary} "
  '^../../bin/relay '
  '^/users/Liquidz/vhive-snapshare/bin/relay '
  '^/users/Liquidz/vswarm/tools/relay/server '
  '^/usr/local/bin/demux-snapshotter '
  '^/usr/local/bin/http-address-resolver$'
  '^/usr/local/bin/firecracker-containerd '
  '^containerd-shim-aws-firecracker '
  '^/usr/local/bin/firecracker '
)
for signal in TERM KILL; do
  for pattern in "${patterns[@]}"; do
    pids=$(pgrep -f "$pattern" || true)
    if [[ -n $pids ]]; then
      sudo kill "-$signal" $pids
    fi
  done
  [[ $signal == TERM ]] && sleep 3
done

if [[ $clean_local_snapshot_cache == 1 ]]; then
  # This is deliberately exact and opt-in. Callers use it to enforce a cold
  # cache between samples after preserving any required full-cache staging.
  sudo rm -rf -- /users/Liquidz/snapshots
fi

fccd_log=$runtime_dir/fccd.$label.log
demux_log=$runtime_dir/demux.$label.log
resolver_log=$runtime_dir/resolver.$label.log
relay_log=$runtime_dir/relay.$label.log

tmux new-session -d -s fccd_snapshare \
  "sudo env PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin /usr/local/bin/firecracker-containerd --config /etc/firecracker-containerd/config.toml > '$fccd_log' 2>&1"
for _ in $(seq 1 30); do
  [[ -S /run/firecracker-containerd/containerd.sock ]] && break
  sleep 1
done
[[ -S /run/firecracker-containerd/containerd.sock ]]

tmux new-session -d -s resolver_snapshare \
  "sudo /usr/local/bin/http-address-resolver > '$resolver_log' 2>&1"
tmux new-session -d -s demux_snapshare \
  "sudo /usr/local/bin/demux-snapshotter --config /etc/demux-snapshotter/config.toml > '$demux_log' 2>&1"
for _ in $(seq 1 30); do
  [[ -S /var/lib/demux-snapshotter/snapshotter.sock ]] && break
  sleep 1
done
[[ -S /var/lib/demux-snapshotter/snapshotter.sock ]]

relay_args=(
  "-endpoint=$relay_endpoint:8080"
  -snapshots="$snapshot_mode"
  -ss=proxy
  -chunkSize="$chunk_size"
  -cacheSize="$cache_size"
  -upf
  -ws
  -security="$security_mode"
  -j="$threads"
  -netPoolSize="$net_pool_size"
  -vethPrefix="$veth_prefix"
  -clonePrefix="$clone_prefix"
  -vmMemSizeMib="$vm_mib"
  -cacheSnaps="$([[ $reuse_local_cache == 1 ]] && echo true || echo false)"
  -hostIface="$host_iface"
  "-minioCredentials=$minio_host:$minio_port;minio;minio123"
  "-freshSourceDelay=${fresh_source_delay_seconds}s"
  -dbg
)
[[ -z $dns_nameservers ]] || relay_args+=("-dnsNameservers=$dns_nameservers")
[[ $enable_chunking == 0 ]] || relay_args+=(-chunking)
[[ $enable_ws_coalescing == 0 ]] || relay_args+=(-wsCoalescing)
[[ $enable_lazy == 0 ]] || relay_args+=(-lazy)
[[ $enable_base_snap == 0 ]] || relay_args+=(-baseSnap)
[[ $enable_ws_compression == 0 ]] || relay_args+=(-wsCompression)
[[ $enable_chunk_compression == 0 ]] || relay_args+=(-chunkCompression)
if [[ $enable_ws_compression == 1 || $enable_chunk_compression == 1 ]]; then
  relay_args+=("-zstdLevel=$zstd_level" "-zstdFrameSize=$zstd_frame_size" "-zstdFetchers=$zstd_fetchers")
fi
[[ $clean_after_invocation == 0 ]] || relay_args+=(-clean)
if [[ $recording == 1 ]]; then
  relay_args+=(-wsRecording)
fi
(( ws_profile_invocations == 0 )) || relay_args+=("-wsProfileInvocations=$ws_profile_invocations")

printf -v relay_cmd ' %q' "$relay_binary" "${relay_args[@]}"
tmux new-session -d -s relay_snapshare \
  "cd '$relay_dir' && sudo env HOME='$HOME' PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin$relay_cmd > '$relay_log' 2>&1"

for _ in $(seq 1 "$ready_timeout_seconds"); do
  ready=0
  if ss -ltn | grep -q ':8080 '; then
    if [[ $enable_chunking == 0 || $security_mode == none || $security_mode == full-dedup || $security_mode == full ]] \
      || { grep -q 'Loaded chunk hashes for' "$relay_log" 2>/dev/null \
        && grep -q 'Loaded rootfs chunk hashes' "$relay_log" 2>/dev/null; }; then
      ready=1
    fi
  fi
  if [[ $ready == 1 ]]; then
    ln -sfn "$(basename "$relay_log")" "$runtime_dir/relay.log"
    printf 'SNAPSHARE_WORKER_READY recording=%s vm_mib=%s cache_size=%s reuse_local_cache=%s clean_local_snapshot_cache=%s snapshot_mode=%s security_mode=%s relay_endpoint=%s host_iface=%s minio=%s:%s threads=%s net_pool_size=%s veth_prefix=%s clone_prefix=%s dns_nameservers=%s clean_after_invocation=%s chunking=%s chunk_size=%s ws_coalescing=%s lazy=%s base_snap=%s ws_compression=%s chunk_compression=%s zstd_level=%s zstd_frame_size=%s zstd_fetchers=%s ws_profile_invocations=%s fresh_source_delay_seconds=%s label=%s relay_log=%s\n' \
      "$recording" "$vm_mib" "$cache_size" "$reuse_local_cache" "$clean_local_snapshot_cache" \
      "$snapshot_mode" "$security_mode" "$relay_endpoint" "$host_iface" "$minio_host" "$minio_port" "$threads" "$net_pool_size" "$veth_prefix" "$clone_prefix" "$dns_nameservers" "$clean_after_invocation" \
      "$enable_chunking" "$chunk_size" "$enable_ws_coalescing" "$enable_lazy" "$enable_base_snap" \
      "$enable_ws_compression" "$enable_chunk_compression" "$zstd_level" "$zstd_frame_size" "$zstd_fetchers" "$ws_profile_invocations" "$fresh_source_delay_seconds" \
      "$label" "$relay_log"
    exit 0
  fi
  sleep 1
done

echo "SnapShare worker did not finish provenance initialization" >&2
tail -n 80 "$relay_log" >&2 || true
exit 4
