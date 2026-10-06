#!/usr/bin/env bash
set -euo pipefail

script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
workspace_dir=$(cd -- "$script_dir/.." && pwd)
nodes_env=${NODES_ENV:-$workspace_dir/colocation-infra/cluster/nodes.20260830.figure9-16-c6620-3node.env}
# shellcheck source=/dev/null
source "$nodes_env"

stamp=${STAMP:-20260830-direct}
resume_inputs=${RESUME_INPUTS:-0}
seed_template_container=${SEED_TEMPLATE_CONTAINER:-}
regenerate_direct_profiles=${REGENERATE_DIRECT_PROFILES:-}
ws_capture_protocol=${WS_CAPTURE_PROTOCOL:-restore-union}
restore_union_base_snap=${RESTORE_UNION_BASE_SNAP:-0}
restore_union_base_source=${RESTORE_UNION_BASE_SOURCE:-legacy}
restore_union_clean_local=${RESTORE_UNION_CLEAN_LOCAL:-0}
restore_union_reuse_local=${RESTORE_UNION_REUSE_LOCAL:-1}
restore_union_pre_clean_local=${RESTORE_UNION_PRE_CLEAN_LOCAL:-0}
result_root=${RESULT_ROOT:-$script_dir/results/figure9-16/${stamp}_c6620_inputs_r1}
workloads_json=${FIGURE_WORKLOADS_JSON:-$script_dir/configs/figure9_16/workloads.json}
direct_requests_json=${FIGURE_DIRECT_REQUESTS_JSON:-$script_dir/configs/figure9_16/direct_requests.json}
legacy_root=${LEGACY_REMOTE_ROOT:-$BACKEND_MOUNT/figure9-16/legacy-source}
images_dir=${FIGURE_IMAGES_DIR:-/users/Liquidz/images}
legacy_manifest=${LEGACY_SOURCE_MANIFEST:-$script_dir/results/figure9-16/20260827_legacy_source_inventory.relative.sha256}
images_manifest=${FIGURE_IMAGES_MANIFEST:-$script_dir/results/figure9-16/20260827_figure_images_inventory.sha256}
data_dir=${TEMPLATE_DATA_DIR:-/mnt/snapshare-zstd-streaming/figure9-16/template-none-$stamp}
container=${TEMPLATE_CONTAINER:-snapshare-fig916-template-$stamp}
port=${TEMPLATE_PORT:-9400}
relay_binary=${FIGURE_RELAY_BINARY:-/users/Liquidz/vhive-snapshare/bin/relay-figure9-16-direct-all}
local_relay_binary=${FIGURE_LOCAL_RELAY_BINARY:-$workspace_dir/.dist/vhive-figure9-16/bin/relay-figure9-16-direct-all}
vhive_source_dir=${FIGURE_VHIVE_SOURCE_DIR:-$workspace_dir/.dist/vhive-figure9-16}
converter_binary=${FIGURE_CONVERTER_BINARY:-/users/Liquidz/vhive-snapshare/bin/snapshot-converter-figure9-16}
runtime_dir=${FIGURE_RUNTIME_DIR:-/users/Liquidz/figure9-16/runtime}
remote_helper=/users/Liquidz/figure9-16/restart-worker.sh
worker=${WORKER_NODES[0]}
worker_ip=${WORKER_PRIVATE_IPS[0]}
backend=${BACKEND_NODE}
backend_ip=${BACKEND_PRIVATE_IP}
loader=${LOADER_NODE:-$BACKEND_NODE}
ssh_opts=(-A -oStrictHostKeyChecking=no -oServerAliveInterval=20 -oServerAliveCountMax=6)
expected_workloads=$(jq -r '.workloads | length' "$workloads_json")

[[ ${EXPERIMENT_ID:-} == figure9-16-direct-er020-er069-er032 ]] \
  || { echo "refusing non-figure9-16 experiment" >&2; exit 2; }
[[ $worker == "$AUTHORIZED_CONTROL_NODE" && $backend == "$AUTHORIZED_BACKEND_NODE" ]] || exit 2
[[ $worker_ip == "$AUTHORIZED_CONTROL_PRIVATE_IP" && $backend_ip == "$AUTHORIZED_BACKEND_PRIVATE_IP" ]] || exit 2
[[ $loader != "$worker" && $loader != "$backend" ]] || exit 2
[[ $stamp =~ ^[0-9]{8}([a-z0-9-]+)?$ ]] || exit 2
[[ $resume_inputs == 0 || $resume_inputs == 1 ]] || exit 2
[[ $port =~ ^[1-9][0-9]*$ && $port -lt 65536 ]] || exit 2
[[ $container =~ ^snapshare-fig916-template-[a-zA-Z0-9-]+$ ]] || exit 2
[[ -z $seed_template_container || $seed_template_container =~ ^snapshare-fig916-template-[a-zA-Z0-9-]+$ ]] || exit 2
[[ -z $regenerate_direct_profiles || $regenerate_direct_profiles =~ ^[a-zA-Z0-9._-]+(,[a-zA-Z0-9._-]+)*$ ]] || exit 2
[[ $ws_capture_protocol == restore-union || $ws_capture_protocol == paper-aligned ]] || exit 2
[[ $restore_union_base_snap == 0 || $restore_union_base_snap == 1 ]] || exit 2
[[ $restore_union_base_source == legacy || $restore_union_base_source == current ]] || exit 2
[[ $restore_union_clean_local == 0 || $restore_union_clean_local == 1 ]] || exit 2
[[ $restore_union_reuse_local == 0 || $restore_union_reuse_local == 1 ]] || exit 2
[[ $restore_union_pre_clean_local == 0 || $restore_union_pre_clean_local == 1 ]] || exit 2
if [[ $restore_union_pre_clean_local == 1 && $restore_union_clean_local == 1 ]]; then
  echo "RESTORE_UNION_PRE_CLEAN_LOCAL and RESTORE_UNION_CLEAN_LOCAL are mutually exclusive" >&2
  exit 2
fi
if [[ $ws_capture_protocol == paper-aligned && -n $seed_template_container ]]; then
  echo "paper-aligned capture cannot seed the post-request direct-all corpus" >&2
  exit 2
fi
[[ $data_dir == /mnt/snapshare-zstd-streaming/figure9-16/* ]] || exit 2
[[ $legacy_root == /mnt/snapshare-zstd-streaming/figure9-16/* ]] || exit 2
[[ $expected_workloads =~ ^[1-9][0-9]*$ ]] || exit 2
[[ $(jq -r '.requests | length' "$direct_requests_json") == "$expected_workloads" ]] || exit 2
[[ -s $legacy_manifest && -s $images_manifest && -s $direct_requests_json ]] \
  || { echo "source inventory or direct request manifest is absent" >&2; exit 3; }
if [[ -e $result_root ]]; then
  [[ $resume_inputs == 1 && ! -e $result_root/INPUTS_COMPLETE ]] \
    || { echo "refusing existing or completed result root: $result_root" >&2; exit 3; }
fi
mkdir -p "$result_root/platform" "$result_root/current-corpus"
launcher=$result_root/launcher.log

log() { printf '%s %s\n' "$(date -Is)" "$*" | tee -a "$launcher"; }

verify_cpu() {
  local host=$1 output=$2
  ssh "${ssh_opts[@]}" "$host" 'bash -s -- verify-2.1ghz' \
    <"$script_dir/configure_c6620_cpu_baseline.sh" >"$output"
}

mc_exec() {
  local command=$1
  {
    printf '%s\n' 'set -e'
    printf '%s\n' 'mc alias set eval http://127.0.0.1:9000 minio minio123 >/dev/null'
    printf '%s\n' "$command"
  } | ssh "${ssh_opts[@]}" "$backend" "sudo docker exec -i '$container' sh"
}

wait_object() {
  local key=$1 deadline=$((SECONDS + 3600))
  while (( SECONDS < deadline )); do
    if mc_exec "mc stat eval/snapshots/$key >/dev/null 2>&1"; then return 0; fi
    sleep 2
  done
  return 1
}

stage_assets() {
  log "verify frozen direct relay, converter, restart helper, and loader invoker"
  local relay_sha converter_sha helper_sha invoker_sha remote_hashes loader_hash
	relay_sha=$(sha256sum "$local_relay_binary" | awk '{print $1}')
  converter_sha=$(sha256sum "$workspace_dir/.dist/vhive-figure9-16/bin/snapshot-converter-figure9-16" | awk '{print $1}')
  helper_sha=$(sha256sum "$script_dir/restart_snapshare_xl170_worker.sh" | awk '{print $1}')
  invoker_sha=$(sha256sum "$workspace_dir/.dist/vswarm-direct-invoker/tools/direct-invoker/direct-invoker" | awk '{print $1}')
  remote_hashes=$(ssh "${ssh_opts[@]}" "$worker" \
    "sha256sum '$relay_binary' '$converter_binary' '$remote_helper' 2>/dev/null | awk '{print \$1}'" || true)
  loader_hash=$(ssh "${ssh_opts[@]}" "$loader" \
    "sha256sum /users/Liquidz/figure9-16/bin/direct-invoker 2>/dev/null | awk '{print \$1}'" || true)
  [[ $remote_hashes == "$relay_sha"$'\n'"$converter_sha"$'\n'"$helper_sha" ]] \
    || { echo "worker assets differ from the staged direct-path build" >&2; exit 5; }
  [[ $loader_hash == "$invoker_sha" ]] \
    || { echo "loader direct invoker differs from the staged build" >&2; exit 5; }
  ssh "${ssh_opts[@]}" "$worker" \
    "sha256sum '$relay_binary' '$converter_binary' '$remote_helper'" \
    >"$result_root/binaries.sha256"
  ssh "${ssh_opts[@]}" "$loader" \
    "sha256sum /users/Liquidz/figure9-16/bin/direct-invoker" \
    >"$result_root/direct-invoker.sha256"
}

verify_staged_legacy() {
  log "verify staged legacy ZIP"
  local local_zip_sha remote_zip_sha
  local_zip_sha=$(sha256sum "$workspace_dir/snapshare-5.01-extracted/snapshare/26_05_01_snapshots/snapshots.zip" | awk '{print $1}')
  remote_zip_sha=$(ssh "${ssh_opts[@]}" "$backend" "sha256sum '$legacy_root/snapshots.zip'" | awk '{print $1}')
  [[ $local_zip_sha == "$remote_zip_sha" ]] || { echo "legacy ZIP differs" >&2; exit 5; }
  printf '%s  %s\n' "$remote_zip_sha" "$legacy_root/snapshots.zip" >"$result_root/legacy-transfer-check.log"
}

verify_staged_images() {
  log "wait for and verify the exact 17-workload image inventory"
  local deadline=$((SECONDS + 3600)) image_count=0
  while (( SECONDS < deadline )); do
    image_count=$(ssh "${ssh_opts[@]}" "$worker" \
      "find '$images_dir' -mindepth 2 -maxdepth 2 -type f -name container.tar | wc -l")
    [[ $image_count == 17 ]] && break
    sleep 5
  done
  [[ $image_count == 17 ]] \
    || { echo "timed out waiting for 17 workload images (found $image_count)" >&2; exit 5; }
  ssh "${ssh_opts[@]}" "$worker" \
    "cd '$images_dir' && { sha256sum rootfs.tar; find . -mindepth 2 -maxdepth 2 -type f -name container.tar -print0 | sort -z | xargs -0 sha256sum; }" \
    >"$result_root/images.remote.sha256"
  cmp -s "$images_manifest" "$result_root/images.remote.sha256" \
    || { echo "remote image inventory differs from frozen inventory" >&2; exit 5; }
}

start_template_minio() {
  log "start isolated unchunked template MinIO"
  if [[ $resume_inputs == 1 ]] && ssh "${ssh_opts[@]}" "$backend" \
    "test \"\$(sudo docker inspect -f '{{.State.Running}} {{range .Mounts}}{{if eq .Destination \"/data\"}}{{.Source}}{{end}}{{end}}' '$container')\" = 'true $data_dir'"; then
    mc_exec 'mc ready eval >/dev/null 2>&1'
    log "reuse running isolated template MinIO"
    return
  fi
  ssh "${ssh_opts[@]}" "$backend" bash -s -- "$data_dir" "$container" "$backend_ip" "$port" <<'REMOTE'
set -euo pipefail
data_dir=$1
container=$2
backend_ip=$3
port=$4
test ! -e "$data_dir"
! sudo docker inspect "$container" >/dev/null 2>&1
! ss -ltn | awk '{print $4}' | grep -Eq "(^|:)$port$"
sudo install -d -o 1000 -g 1000 "$data_dir"
sudo docker run -d --name "$container" -p "$backend_ip:$port:9000" \
  -e MINIO_ROOT_USER=minio -e MINIO_ROOT_PASSWORD=minio123 \
  -v "$data_dir:/data" quay.io/minio/minio:RELEASE.2024-12-18T13-15-44Z server /data
REMOTE
  for _ in $(seq 1 120); do
    if mc_exec 'mc ready eval >/dev/null 2>&1'; then
      mc_exec 'mc mb eval/snapshots'
      return 0
    fi
    sleep 1
  done
  return 4
}

import_legacy() {
  local legacy_count
  legacy_count=$(jq '[.workloads[] | select(.source=="legacy")] | length' "$workloads_json")
	if [[ $ws_capture_protocol == paper-aligned ]]; then
		# Paper-aligned profiles must share a base created from the current
		# rootfs/runtime environment.  Importing the historical base hid a stale
		# guest stargz-socket dependency and also mixed two base provenances.
		# Every selected workload is regenerated, so no legacy revision is needed.
		log "paper-aligned capture starts with an empty namespace; generate one current base locally"
		return
	fi
  if [[ $restore_union_base_source == current ]]; then
    [[ $restore_union_base_snap == 1 ]] \
      || { echo "RESTORE_UNION_BASE_SOURCE=current requires RESTORE_UNION_BASE_SNAP=1" >&2; exit 2; }
    [[ $legacy_count == 0 ]] \
      || { echo "current-base restore-union cannot mix historical function snapshots" >&2; exit 2; }
    [[ $(jq '[.workloads[].vm_mib] | unique | length' "$workloads_json") == 1 ]] \
      || { echo "current-base restore-union requires one vm_mib per isolated namespace; split mixed-memory manifests by tier" >&2; exit 2; }
    log "restore-union starts with an empty namespace; first source creates one current base"
    return
  fi
  log "import reusable legacy snapshots plus base as immutable inputs legacy_count=$legacy_count"
  if [[ $resume_inputs == 1 && -s $result_root/LEGACY_IMPORTED_VERIFIED ]]; then
    log "reuse checkpointed legacy plus base import legacy_count=$legacy_count"
    return
  fi
  if [[ $resume_inputs == 1 ]]; then
    local legacy_ready=true snapshot object
    while read -r snapshot; do
      for object in info_file snap_file mem_file; do
        if ! mc_exec "mc stat eval/snapshots/$snapshot/$object >/dev/null 2>&1"; then
          legacy_ready=false
        fi
      done
      if [[ $snapshot != base ]] && ! mc_exec \
        "mc stat eval/snapshots/$snapshot/working_set_pages >/dev/null 2>&1"; then
        legacy_ready=false
      fi
      if mc_exec "mc stat eval/snapshots/$snapshot/recipe_file >/dev/null 2>&1"; then
        legacy_ready=false
      fi
    done < <(printf 'base\n'; jq -r '.workloads[] | select(.source=="legacy") | .snapshot' "$workloads_json")
    if [[ $legacy_ready == true ]]; then
      date -Is >"$result_root/LEGACY_IMPORTED_VERIFIED"
      log "reuse verified legacy plus base import legacy_count=$legacy_count"
      return
    fi
    echo "resume requested but legacy import is incomplete" >&2
    exit 5
  fi
  mapfile -t legacy_snapshots < <(printf 'base\n'; jq -r '.workloads[] | select(.source=="legacy") | .snapshot' "$workloads_json")
  ssh "${ssh_opts[@]}" "$backend" bash -s -- "$legacy_root" "$container" "$legacy_count" "${legacy_snapshots[@]}" <<'REMOTE' \
    2>&1 | tee "$result_root/legacy-import.log"
set -euo pipefail
legacy_root=$1
container=$2
legacy_count=$3
shift 3
archive=$legacy_root/snapshots.zip
test -f "$archive"
test "$#" -eq "$((legacy_count + 1))"
sudo docker exec "$container" sh -lc 'mc alias set eval http://127.0.0.1:9000 minio minio123 >/dev/null'
for snapshot in "$@"; do
  prefix=snapshots/$snapshot
  for object in info_file snap_file working_set_pages; do
    if unzip -Z1 "$archive" "$prefix/$object" 2>/dev/null | grep -Fxq "$prefix/$object"; then
      unzip -p "$archive" "$prefix/$object" | \
      sudo docker exec -i "$container" sh -lc \
        "mc pipe eval/snapshots/$snapshot/$object >/dev/null"
    fi
  done
  if unzip -Z1 "$archive" "$prefix/mem_file" 2>/dev/null | grep -Fxq "$prefix/mem_file"; then
    unzip -p "$archive" "$prefix/mem_file" | \
    sudo docker exec -i "$container" sh -lc \
      "mc pipe eval/snapshots/$snapshot/mem_file >/dev/null"
  else
    unzip -Z1 "$archive" "$prefix/mem_file.gz" 2>/dev/null | grep -Fxq "$prefix/mem_file.gz"
    unzip -p "$archive" "$prefix/mem_file.gz" | gzip -dc | sudo docker exec -i "$container" sh -lc \
      "mc pipe eval/snapshots/$snapshot/mem_file >/dev/null"
  fi
done
REMOTE
  [[ $(mc_exec "mc ls eval/snapshots | wc -l") -eq $((legacy_count + 1)) ]]
  [[ $(mc_exec "mc find eval/snapshots --name recipe_file | wc -l") -eq 0 ]]
  date -Is >"$result_root/LEGACY_IMPORTED_VERIFIED"
}

profile_is_regenerated() {
  local profile=$1 item
  [[ -n $regenerate_direct_profiles ]] || return 1
  IFS=, read -r -a items <<<"$regenerate_direct_profiles"
  for item in "${items[@]}"; do
    [[ $profile == "$item" ]] && return 0
  done
  return 1
}

seed_reusable_direct_profiles() {
	if [[ $ws_capture_protocol == paper-aligned ]]; then
		log "paper-aligned capture regenerates every selected profile; skip reusable direct corpus"
		return
	fi
  [[ -n $seed_template_container ]] || return 0
  log "seed unchanged direct-path snapshots from immutable template=$seed_template_container"
  ssh "${ssh_opts[@]}" "$backend" \
    "test \"\$(sudo docker inspect -f '{{.State.Running}}' '$seed_template_container')\" = true" </dev/null
  local profile snapshot copied=0
  while read -r profile; do
    [[ -n $profile ]] || continue
    if profile_is_regenerated "$profile"; then
      log "leave direct profile absent for regeneration profile=$profile"
      continue
    fi
    snapshot=$(jq -r --arg p "$profile" '.workloads[] | select(.profile==$p) | .snapshot' "$workloads_json")
    if [[ $resume_inputs == 1 ]] && mc_exec \
      "mc stat eval/snapshots/$snapshot/mem_file >/dev/null 2>&1"; then
      ssh "${ssh_opts[@]}" "$backend" bash -s -- \
        "$seed_template_container" "$backend_ip" "$port" "$snapshot" <<'REMOTE'
set -euo pipefail
source_container=$1
backend_ip=$2
destination_port=$3
snapshot=$4
sudo docker exec "$source_container" sh -lc '
  set -e
  mc alias set source http://127.0.0.1:9000 minio minio123 >/dev/null
  mc alias set destination http://'"$backend_ip:$destination_port"' minio minio123 >/dev/null
  test -z "$(mc diff source/snapshots/'"$snapshot"' destination/snapshots/'"$snapshot"')"
'
REMOTE
      copied=$((copied + 1))
      log "resume: reuse byte-identical seeded direct profile=$profile"
      continue
    fi
    ssh "${ssh_opts[@]}" "$backend" bash -s -- \
      "$seed_template_container" "$backend_ip" "$port" "$snapshot" <<'REMOTE'
set -euo pipefail
source_container=$1
backend_ip=$2
destination_port=$3
snapshot=$4
sudo docker exec "$source_container" sh -lc '
  set -e
  mc alias set source http://127.0.0.1:9000 minio minio123 >/dev/null
  mc alias set destination http://'"$backend_ip:$destination_port"' minio minio123 >/dev/null
  mc stat source/snapshots/'"$snapshot"'/mem_file >/dev/null
  ! mc stat destination/snapshots/'"$snapshot"'/mem_file >/dev/null 2>&1
  mc cp --quiet --recursive source/snapshots/'"$snapshot"'/ destination/snapshots/'"$snapshot"'/
  test -z "$(mc diff source/snapshots/'"$snapshot"' destination/snapshots/'"$snapshot"')"
'
REMOTE
    copied=$((copied + 1))
  done < <(jq -r '.workloads[] | select(.source!="legacy") | .profile' "$workloads_json")
  log "seeded reusable direct profiles count=$copied"
}

restart_recording_worker() {
	local profile=$1 vm_mib label=$2 profile_invocations=0 fresh_source_delay_seconds
	local clean_local=$restore_union_clean_local reuse_local=$restore_union_reuse_local
	local base_snap=$restore_union_base_snap
	vm_mib=$(jq -r --arg p "$profile" '.workloads[] | select(.profile==$p) | .vm_mib' "$workloads_json")
	fresh_source_delay_seconds=$(jq -r --arg p "$profile" '.workloads[] | select(.profile==$p) | (.fresh_source_delay_seconds // 10)' "$workloads_json")
	[[ $fresh_source_delay_seconds =~ ^[0-9]+$ ]] || { echo "invalid source delay for $profile" >&2; exit 5; }
	if [[ $ws_capture_protocol == paper-aligned ]]; then
		profile_invocations=5
		clean_local=1
		reuse_local=0
		base_snap=1
	fi
	if [[ $ws_capture_protocol == restore-union && $restore_union_pre_clean_local == 1 ]]; then
		ssh "${ssh_opts[@]}" "$worker" \
			"CLEAN_LOCAL_SNAPSHOT_CACHE=1 SNAPSHARE_IMAGES_DIR='$images_dir' SNAPSHARE_READY_TIMEOUT_SECONDS=600 SNAPSHARE_RELAY_BINARY='$relay_binary' SNAPSHARE_RUNTIME_DIR='$runtime_dir' SNAPSHARE_RELAY_DIR='/users/Liquidz/vhive-zstd-streaming/cmd/relay' SNAPSHARE_RELAY_ENDPOINT='$worker_ip' SNAPSHARE_MINIO_HOST='$backend_ip' SNAPSHARE_MINIO_PORT='$port' SNAPSHARE_THREADS=8 SNAPSHARE_NET_POOL_SIZE=1 SNAPSHARE_VETH_PREFIX=172.26 SNAPSHARE_CLONE_PREFIX=172.27 SNAPSHARE_DNS_NAMESERVERS='$backend_ip' SNAPSHARE_CLEAN_AFTER_INVOCATION=0 SNAPSHARE_CHUNKING=0 SNAPSHARE_CHUNK_SIZE=4096 SNAPSHARE_WS_COALESCING=0 SNAPSHARE_LAZY=0 SNAPSHARE_BASE_SNAP=0 SNAPSHARE_WS_PROFILE_INVOCATIONS=0 SNAPSHARE_FRESH_SOURCE_DELAY_SECONDS='$fresh_source_delay_seconds' '$remote_helper' 0 '$vm_mib' '$label-preclean' 1000000 0 remote none" </dev/null
	fi
	ssh "${ssh_opts[@]}" "$worker" \
		"CLEAN_LOCAL_SNAPSHOT_CACHE='$clean_local' SNAPSHARE_IMAGES_DIR='$images_dir' SNAPSHARE_READY_TIMEOUT_SECONDS=600 SNAPSHARE_RELAY_BINARY='$relay_binary' SNAPSHARE_RUNTIME_DIR='$runtime_dir' SNAPSHARE_RELAY_DIR='/users/Liquidz/vhive-zstd-streaming/cmd/relay' SNAPSHARE_RELAY_ENDPOINT='$worker_ip' SNAPSHARE_MINIO_HOST='$backend_ip' SNAPSHARE_MINIO_PORT='$port' SNAPSHARE_THREADS=8 SNAPSHARE_NET_POOL_SIZE=1 SNAPSHARE_VETH_PREFIX=172.26 SNAPSHARE_CLONE_PREFIX=172.27 SNAPSHARE_DNS_NAMESERVERS='$backend_ip' SNAPSHARE_CLEAN_AFTER_INVOCATION=0 SNAPSHARE_CHUNKING=0 SNAPSHARE_CHUNK_SIZE=4096 SNAPSHARE_WS_COALESCING=0 SNAPSHARE_LAZY=0 SNAPSHARE_BASE_SNAP='$base_snap' SNAPSHARE_WS_PROFILE_INVOCATIONS='$profile_invocations' SNAPSHARE_FRESH_SOURCE_DELAY_SECONDS='$fresh_source_delay_seconds' '$remote_helper' 1 '$vm_mib' '$label' 1000000 '$reuse_local' remote none" </dev/null
}

run_source_with_retries() {
  local profile=$1 snapshot=$2 attempt remote_exists local_exists
  for attempt in 1 2 3; do
    if SNAPSHARE_MINIO_CONTAINER=$container SNAPSHARE_READY_OBJECT=mem_file \
      RESULT_CSV=$result_root/current-corpus/calls.csv RUN_INDEX=0 NODES_ENV=$nodes_env \
      "$script_dir/run_figure9_16_call.sh" "$profile" source \
      2>&1 | tee "$result_root/current-corpus/$profile.source.attempt-$attempt.log" \
        "$result_root/current-corpus/$profile.source.log"; then
      return 0
    fi
    remote_exists=false
    local_exists=false
    mc_exec "mc stat eval/snapshots/$snapshot/mem_file >/dev/null 2>&1 && \
      mc stat eval/snapshots/$snapshot/snap_file >/dev/null 2>&1 && \
      mc stat eval/snapshots/$snapshot/info_file >/dev/null 2>&1 && \
      mc stat eval/snapshots/$snapshot/working_set_pages >/dev/null 2>&1" && remote_exists=true
    ssh "${ssh_opts[@]}" "$worker" "test -d '/users/Liquidz/snapshots/$snapshot'" </dev/null \
      && local_exists=true
    [[ $remote_exists == false && $local_exists == false ]] || {
      echo "failed source left a partial snapshot: $profile remote=$remote_exists local=$local_exists" >&2
      return 5
    }
    log "retry source after clean pre-snapshot failure profile=$profile attempt=$attempt"
    sleep 5
  done
  echo "source failed after three clean attempts: $profile" >&2
  return 5
}

record_direct_profiles() {
  local profiles profile label invocation snapshot signature_before signature_after remote_exists local_exists uffd_line task_line
  if [[ ! -e $result_root/current-corpus/calls.csv ]]; then
    printf '%s\n' 'timestamp,profile,stage,run_index,e2e_ns,status' \
      >"$result_root/current-corpus/calls.csv"
  fi
	if [[ $ws_capture_protocol == paper-aligned ]]; then
		profiles=$(jq -r '.workloads[].profile' "$workloads_json")
	else
		profiles=$(jq -r '.workloads[] | select(.source!="legacy") | .profile' "$workloads_json")
	fi
  while read -r profile; do
    [[ -n $profile ]] || continue
    if [[ -n $seed_template_container ]] && ! profile_is_regenerated "$profile"; then
      log "keep immutable seeded direct profile=$profile"
      continue
    fi
		snapshot=$(jq -r --arg p "$profile" '.workloads[] | select(.profile==$p) | .snapshot' "$workloads_json")
		if [[ $ws_capture_protocol == restore-union ]] &&
		   [[ -s $result_root/current-corpus/$profile.learning.1.complete &&
		      -s $result_root/current-corpus/$profile.learning.2.complete &&
		      -s $result_root/current-corpus/$profile.learning.3.complete &&
		      -s $result_root/current-corpus/$profile.learning.4.complete &&
		      -s $result_root/current-corpus/$profile.learning.5.complete ]] &&
		   mc_exec "mc stat eval/snapshots/$snapshot/mem_file >/dev/null 2>&1 && \
		     mc stat eval/snapshots/$snapshot/snap_file >/dev/null 2>&1 && \
		     mc stat eval/snapshots/$snapshot/info_file >/dev/null 2>&1 && \
		     mc stat eval/snapshots/$snapshot/working_set_pages >/dev/null 2>&1"; then
			log "resume: skip fully completed profile=$profile"
			continue
		fi
		label=$stamp-$profile-recording
		restart_recording_worker "$profile" "$label" | tee "$result_root/current-corpus/$profile.restart.log"
		if [[ $ws_capture_protocol == paper-aligned ]]; then
			! mc_exec "mc stat eval/snapshots/$snapshot/mem_file >/dev/null 2>&1" \
				|| { echo "paper-aligned snapshot already exists remotely: $profile" >&2; exit 5; }
			! ssh "${ssh_opts[@]}" "$worker" "test -e '/users/Liquidz/snapshots/$snapshot'" </dev/null \
				|| { echo "paper-aligned snapshot already exists locally: $profile" >&2; exit 5; }
			for invocation in 0 1 2 3 4; do
				if [[ -s $result_root/current-corpus/$profile.profile.$invocation.complete ]]; then
					echo "paper-aligned sessions cannot resume in the middle of a five-request window" >&2
					exit 5
				fi
				SNAPSHARE_MINIO_CONTAINER=$container SNAPSHARE_READY_OBJECT=mem_file \
					RESULT_CSV=$result_root/current-corpus/calls.csv RUN_INDEX=$invocation NODES_ENV=$nodes_env \
					"$script_dir/run_figure9_16_call.sh" "$profile" profile \
					2>&1 | tee "$result_root/current-corpus/$profile.profile.$invocation.log"
				date -Is >"$result_root/current-corpus/$profile.profile.$invocation.complete"
			done
			ssh "${ssh_opts[@]}" "$worker" "cat '$runtime_dir/relay.$label.log'" </dev/null \
				>"$result_root/current-corpus/$profile.relay.log"
			[[ $(grep -c "WS_PROFILE_WINDOW_REQUEST revision=$snapshot .* phase=begin" "$result_root/current-corpus/$profile.relay.log") == 5 ]]
			[[ $(grep -c "WS_PROFILE_WINDOW_REQUEST revision=$snapshot .* phase=complete" "$result_root/current-corpus/$profile.relay.log") == 5 ]]
			[[ $(grep -c "WS_PROFILE_WINDOW_COMPLETE revision=$snapshot .* requests=5" "$result_root/current-corpus/$profile.relay.log") == 1 ]]
			[[ $(grep "WS_PROFILE_WINDOW_REQUEST revision=$snapshot .* phase=begin" "$result_root/current-corpus/$profile.relay.log" \
				| sed -n 's/.* vm=\([^ ]*\).*/\1/p' | sort -u | wc -l) == 1 ]]
			uffd_line=$(grep -n "WS_PROFILE_UFFD_READY revision=$snapshot " "$result_root/current-corpus/$profile.relay.log" | head -n1 | cut -d: -f1)
			ready_line=$(grep -n "WS_PROFILE_REQUEST_WINDOW_READY revision=$snapshot " "$result_root/current-corpus/$profile.relay.log" | head -n1 | cut -d: -f1)
			frozen_line=$(grep -n "WS_PROFILE_CAPTURE_FROZEN revision=$snapshot .* requests=5" "$result_root/current-corpus/$profile.relay.log" | head -n1 | cut -d: -f1)
			snapshot_line=$(grep -n "finished commiting snapshot $snapshot" "$result_root/current-corpus/$profile.relay.log" | head -n1 | cut -d: -f1)
			[[ $uffd_line =~ ^[1-9][0-9]*$ && $ready_line =~ ^[1-9][0-9]*$ && $uffd_line -lt $ready_line ]]
			[[ $frozen_line =~ ^[1-9][0-9]*$ && $snapshot_line =~ ^[1-9][0-9]*$ && $frozen_line -lt $snapshot_line ]]
			continue
		fi
		remote_exists=false
    local_exists=false
    mc_exec "mc stat eval/snapshots/$snapshot/mem_file >/dev/null 2>&1 && \
      mc stat eval/snapshots/$snapshot/snap_file >/dev/null 2>&1 && \
      mc stat eval/snapshots/$snapshot/info_file >/dev/null 2>&1 && \
      mc stat eval/snapshots/$snapshot/working_set_pages >/dev/null 2>&1" && remote_exists=true
    ssh "${ssh_opts[@]}" "$worker" "test -d '/users/Liquidz/snapshots/$snapshot'" </dev/null && local_exists=true
    if [[ $remote_exists == true ]]; then
      log "resume: reuse complete remote source snapshot profile=$profile local_present=$local_exists"
    elif [[ $remote_exists == false && $local_exists == false ]]; then
      run_source_with_retries "$profile" "$snapshot"
    else
      echo "source snapshot is only partially present: $profile remote=$remote_exists local=$local_exists" >&2
      exit 5
    fi
    for invocation in 1 2 3 4 5; do
      if [[ -s $result_root/current-corpus/$profile.learning.$invocation.complete ]]; then
        log "resume: keep completed WS learning profile=$profile invocation=$invocation"
        continue
      fi
      signature_before=$(mc_exec "mc stat --json eval/snapshots/$snapshot/working_set_pages 2>/dev/null | sha256sum" || true)
      SNAPSHARE_MINIO_CONTAINER=$container SNAPSHARE_READY_OBJECT=mem_file \
        RESULT_CSV=$result_root/current-corpus/calls.csv RUN_INDEX=$invocation NODES_ENV=$nodes_env \
        "$script_dir/run_figure9_16_call.sh" "$profile" source-local \
        2>&1 | tee "$result_root/current-corpus/$profile.learning.$invocation.log"
      for _ in $(seq 1 120); do
        signature_after=$(mc_exec "mc stat --json eval/snapshots/$snapshot/working_set_pages 2>/dev/null | sha256sum" || true)
        if [[ -n $signature_after && $signature_after != "$signature_before" ]]; then break; fi
        sleep 1
      done
      [[ -n ${signature_after:-} && $signature_after != "$signature_before" ]] \
        || { echo "working-set commit did not advance: $profile learning=$invocation" >&2; exit 5; }
      date -Is >"$result_root/current-corpus/$profile.learning.$invocation.complete"
    done
  done <<<"$profiles"
}

verify_template() {
  log "verify 17-workload unchunked template and record inventory"
  local snapshot object profile vm_mib expected_bytes actual_bytes ws_counts ws_rows ws_unique base_vm_mib base_bytes
  : >"$result_root/template.sha256"
  printf '%s\n' 'snapshot,rows,unique_pfns' >"$result_root/working-set-uniqueness.csv"
  while read -r snapshot; do
    for object in info_file snap_file mem_file; do
      digest=$(mc_exec "set -o pipefail; mc cat eval/snapshots/$snapshot/$object | sha256sum" | awk 'NF {print $1}')
      [[ $digest =~ ^[0-9a-f]{64}$ ]] || exit 5
      printf '%s  %s/%s\n' "$digest" "$snapshot" "$object" >>"$result_root/template.sha256"
    done
    if [[ $snapshot != base ]]; then
      mc_exec "mc stat eval/snapshots/$snapshot/working_set_pages >/dev/null"
      ws_counts=$(mc_exec "mc cat eval/snapshots/$snapshot/working_set_pages" \
        | awk -F, 'NR > 1 && NF && $1 != "" { rows++; seen[$1] = 1 } END { for (pfn in seen) unique++; print rows, unique }')
      read -r ws_rows ws_unique <<<"$ws_counts"
      [[ $ws_rows =~ ^[1-9][0-9]*$ && $ws_unique == "$ws_rows" ]] || {
        echo "working-set PFNs are not unique snapshot=$snapshot rows=${ws_rows:-unknown} unique=${ws_unique:-unknown}" >&2
        exit 5
      }
      printf '%s,%s,%s\n' "$snapshot" "$ws_rows" "$ws_unique" \
        >>"$result_root/working-set-uniqueness.csv"
    fi
  done < <(printf 'base\n'; jq -r '.workloads[].snapshot' "$workloads_json")
	[[ $(wc -l <"$result_root/template.sha256") -eq $(((expected_workloads + 1) * 3)) ]]
	[[ $(awk 'END {print NR-1}' "$result_root/working-set-uniqueness.csv") -eq "$expected_workloads" ]]
  if [[ $restore_union_base_source == current ]]; then
    base_vm_mib=$(jq -r '.workloads[0].vm_mib' "$workloads_json")
    base_bytes=$(mc_exec 'mc stat --json eval/snapshots/base/mem_file' | jq -r '.size')
    [[ $base_bytes == $((base_vm_mib * 1024 * 1024)) ]] || {
      echo "current base mem_file size mismatch vm_mib=$base_vm_mib bytes=$base_bytes" >&2
      exit 5
    }
    printf 'vm_mib=%s\nmem_file_bytes=%s\n' "$base_vm_mib" "$base_bytes" \
      >"$result_root/current-base-size.txt"
  fi
  printf '%s\n' 'profile,snapshot,vm_mib,mem_file_bytes' >"$result_root/mem-file-sizes.csv"
  while IFS=$'\t' read -r profile snapshot vm_mib; do
    expected_bytes=$((vm_mib * 1024 * 1024))
    actual_bytes=$(mc_exec "mc stat --json eval/snapshots/$snapshot/mem_file" | jq -r '.size')
    [[ $actual_bytes == "$expected_bytes" ]] || {
      echo "mem_file size mismatch profile=$profile snapshot=$snapshot expected=$expected_bytes actual=$actual_bytes" >&2
      exit 5
    }
    printf '%s,%s,%s,%s\n' "$profile" "$snapshot" "$vm_mib" "$actual_bytes" \
      >>"$result_root/mem-file-sizes.csv"
  done < <(jq -r '.workloads[] | [.profile,.snapshot,.vm_mib] | @tsv' "$workloads_json")
	[[ $(awk 'END {print NR-1}' "$result_root/mem-file-sizes.csv") -eq "$expected_workloads" ]]
  mc_exec 'mc du eval/snapshots; mc ls --recursive --summarize eval/snapshots' >"$result_root/template.inventory.txt"
  date -Is >"$result_root/INPUTS_COMPLETE"
}

cp "$nodes_env" "$result_root/nodes.env"
cp "$workloads_json" "$result_root/workloads.json"
cp "$direct_requests_json" "$result_root/direct_requests.json"
cp "$script_dir/configs/figure9_16/matrix.json" "$result_root/matrix.json"
cp "$script_dir/configs/figure9_16/config_eval_real_17.json" "$result_root/config_eval_real_17.json"
cp "$legacy_manifest" "$result_root/legacy-source.sha256"
cp "$images_manifest" "$result_root/images-source.sha256"
cp "$script_dir/results/figure9-16/20260827_rootfs_equivalence.txt" "$result_root/rootfs-equivalence.txt"
cp "$0" "$result_root/prepare_figure9_16_inputs.sh"
printf '%s\n' "$ws_capture_protocol" >"$result_root/ws-capture-protocol.txt"
printf 'RESTORE_UNION_BASE_SNAP=%s\nRESTORE_UNION_CLEAN_LOCAL=%s\nRESTORE_UNION_REUSE_LOCAL=%s\nRESTORE_UNION_PRE_CLEAN_LOCAL=%s\nRESTORE_UNION_BASE_SOURCE=%s\n' \
	"$restore_union_base_snap" "$restore_union_clean_local" "$restore_union_reuse_local" \
	"$restore_union_pre_clean_local" "$restore_union_base_source" \
	>"$result_root/restore-union-settings.env"
git -C "$vhive_source_dir" rev-parse HEAD >"$result_root/vhive.commit.txt"
git -C "$vhive_source_dir" status --short --branch >"$result_root/vhive.status.txt"
verify_cpu "$worker" "$result_root/platform/worker.cpu.txt"
verify_cpu "$backend" "$result_root/platform/backend.cpu.txt"
verify_cpu "$loader" "$result_root/platform/loader.cpu.txt"
ssh "${ssh_opts[@]}" "$worker" "tc qdisc show dev ifb0" | tee "$result_root/platform/worker-ifb.tc.txt" \
  | grep -Eq '^qdisc tbf .*rate 10Gbit .*lat 400ms'
! ssh "${ssh_opts[@]}" "$backend" "tc qdisc show dev '$NODE_PRIVATE_INTERFACE'" \
  | tee "$result_root/platform/backend.tc.txt" | grep -q '^qdisc tbf '
verify_staged_legacy
stage_assets
start_template_minio
import_legacy
seed_reusable_direct_profiles
verify_staged_images
record_direct_profiles
verify_template
log "FIGURE_9_16_INPUTS_COMPLETE snapshots=$expected_workloads base=1 regenerated_profiles=${regenerate_direct_profiles:-all} ws_capture_protocol=$ws_capture_protocol profiling_requests_each=5 restore_union_base_snap=$restore_union_base_snap restore_union_base_source=$restore_union_base_source restore_union_clean_local=$restore_union_clean_local restore_union_reuse_local=$restore_union_reuse_local restore_union_pre_clean_local=$restore_union_pre_clean_local"
