#!/usr/bin/env bash
set -euo pipefail

script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
workspace_dir=$(cd -- "$script_dir/.." && pwd)
nodes_env=${NODES_ENV:-$workspace_dir/colocation-infra/cluster/nodes.20260830.figure9-16-c6620-3node.env}
# shellcheck source=/dev/null
source "$nodes_env"

stamp=${STAMP:-20260830-direct}
resume_systems=${RESUME_SYSTEMS:-0}
expected_revisions=${EXPECTED_REVISIONS:-17}
port_base=${CORPUS_PORT_BASE:-9410}
input_root=${INPUT_ROOT:-$script_dir/results/figure9-16/${stamp}_c6620_inputs_r1}
result_root=${RESULT_ROOT:-$script_dir/results/figure9-16/${stamp}_c6620_systems_r1}
matrix_json=${FIGURE_MATRIX_JSON:-$script_dir/configs/figure9_16/matrix.json}
template_container=${TEMPLATE_CONTAINER:-snapshare-fig916-template-$stamp}
backend_mount=${BACKEND_MOUNT:-/mnt/snapshare-zstd-streaming}
images_dir=${FIGURE_IMAGES_DIR:-/users/Liquidz/images}
converter=${FIGURE_CONVERTER_BINARY:-/users/Liquidz/vhive-snapshare/bin/snapshot-converter-figure9-16}
worker=${WORKER_NODES[0]}
backend=${BACKEND_NODE}
backend_ip=${BACKEND_PRIVATE_IP}
ssh_opts=(-A -oStrictHostKeyChecking=no -oServerAliveInterval=20 -oServerAliveCountMax=6)

[[ -s $input_root/INPUTS_COMPLETE && -s $input_root/template.sha256 \
   && -s $input_root/working-set-uniqueness.csv ]] || { echo "input corpus is incomplete" >&2; exit 3; }
[[ $resume_systems == 0 || $resume_systems == 1 ]] || exit 2
[[ $expected_revisions =~ ^[1-9][0-9]*$ ]] || exit 2
[[ $port_base =~ ^[1-9][0-9]*$ && $((port_base + 4)) -lt 65536 ]] || exit 2
[[ $(jq '.workloads | length' "$input_root/workloads.json") == "$expected_revisions" ]] \
  || { echo "input workload count differs from EXPECTED_REVISIONS" >&2; exit 3; }
if [[ -e $result_root ]]; then
  [[ $resume_systems == 1 && ! -e $result_root/SYSTEMS_COMPLETE ]] \
    || { echo "refusing existing or completed result root: $result_root" >&2; exit 3; }
fi
[[ ${EXPERIMENT_ID:-} == figure9-16-direct-er020-er069-er032 ]] || exit 2
[[ $worker == "$AUTHORIZED_CONTROL_NODE" && $backend == "$AUTHORIZED_BACKEND_NODE" ]] || exit 2
mkdir -p "$result_root/converters" "$result_root/inventory" "$result_root/platform"
launcher=$result_root/launcher.log
systems_csv=$result_root/corpora.csv
if [[ ! -e $systems_csv ]]; then
  printf '%s\n' 'corpus_id,port,container,data_dir,security,chunk_size,ws_coalescing' >"$systems_csv"
fi

log() { printf '%s %s\n' "$(date -Is)" "$*" | tee -a "$launcher"; }

mc_exec() {
  local container=$1 command=$2
  {
    printf '%s\n' 'set -e'
    printf '%s\n' 'mc alias set eval http://127.0.0.1:9000 minio minio123 >/dev/null'
    printf '%s\n' "$command"
  } | ssh "${ssh_opts[@]}" "$backend" "sudo docker exec -i '$container' sh"
}

wait_minio() {
  local container=$1
  for _ in $(seq 1 120); do
    if mc_exec "$container" 'mc ready eval >/dev/null 2>&1'; then return 0; fi
    sleep 1
  done
  return 1
}

start_corpus() {
  local corpus_id=$1 port=$2 container=$3 data_dir=$4
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
  wait_minio "$container"
  mc_exec "$container" 'mc mb eval/snapshots'
}

mirror_template() {
  local destination_port=$1
  ssh "${ssh_opts[@]}" "$backend" bash -s -- "$template_container" "$backend_ip" "$destination_port" <<'REMOTE'
set -euo pipefail
template_container=$1
backend_ip=$2
destination_port=$3
sudo docker exec "$template_container" sh -lc '
  set -e
  mc alias set source http://127.0.0.1:9000 minio minio123 >/dev/null
  mc alias set destination http://'"$backend_ip:$destination_port"' minio minio123 >/dev/null
  mc mirror --quiet --overwrite source/snapshots destination/snapshots
  test -z "$(mc diff source/snapshots destination/snapshots)"
'
REMOTE
}

capture_recipe_hashes() {
  local container=$1 destination=$2 snapshot digest
  : >"$destination"
  while read -r snapshot; do
    digest=$(mc_exec "$container" "set -o pipefail; mc cat eval/snapshots/$snapshot/recipe_file | sha256sum" | awk 'NF {print $1}')
    [[ $digest =~ ^[0-9a-f]{64}$ ]] || exit 5
    printf '%s  %s/recipe_file\n' "$digest" "$snapshot" >>"$destination"
  done < <(printf 'base\n'; jq -r '.workloads[].snapshot' "$input_root/workloads.json")
  [[ $(wc -l <"$destination") -eq $((expected_revisions + 1)) ]]
}

capture_inventory_summary() {
  local container=$1 destination=$2
  mc_exec "$container" 'mc du eval/snapshots' >"$destination.new"
  mv -f "$destination.new" "$destination"
}

run_converter() {
  local corpus_id=$1 port=$2 security=$3 chunk_size=$4 ws_coalescing=$5 phase=$6
  # Converter workspaces are disposable, but must be unique to the immutable
  # corpus build. Reusing the old corpus/phase-only name makes an unrelated
  # historical run block a new build at the test-not-exists gate.
  local base=/users/Liquidz/figure9-16/converter-$corpus_id-$stamp-$phase
  local args=(
    -minioURL "$backend_ip:$port" -bucket snapshots -mode "$security"
    -baseDir "$base" -chunkSize "$chunk_size" -workers 4 -allRevisions
    -serviceFanOut=false -lazy -zstdLevel 3 -zstdFrameSize 1048576 -zstdFetchers 8
  )
  if [[ $ws_coalescing == true ]]; then args+=(-wsCoalescing); fi
  if [[ $phase == zstd3 ]]; then
    args+=(-chunkCompression -preserveRecipe)
    if [[ $ws_coalescing == true ]]; then args+=(-wsCompression); fi
  fi
  printf '%q ' "$converter" "${args[@]}" >"$result_root/converters/$corpus_id.$phase.command.txt"
  printf '\n' >>"$result_root/converters/$corpus_id.$phase.command.txt"
  ssh "${ssh_opts[@]}" "$worker" bash -s -- "$base" "$images_dir" "$converter" "${args[@]}" <<'REMOTE'
set -euo pipefail
base=$1
images=$2
converter=$3
shift 3
test ! -e "$base"
mkdir -p "$base/images"
cp -al "$images/." "$base/images/"
exec "$converter" "$@"
REMOTE
}

verify_converter_log() {
  local log_file=$1
  grep -Fq "Found $expected_revisions snapshots to process" "$log_file"
  grep -Fq "Processing all $expected_revisions snapshot revisions" "$log_file"
  ! grep -Eq 'level=(error|panic)|Failed (processing|uploading|materializing)|Conversion incomplete' "$log_file"
}

mapfile -t corpus_rows < <(jq -r '
  [.systems[]] | group_by(.corpus_id)[] |
  .[0].corpus_id as $id |
  ($id + "\t" + (.[0].security) + "\t" + (.[0].chunk_size|tostring) + "\t" +
   (if any(.[]; .ws_coalescing) then "true" else "false" end))
' "$matrix_json")
[[ ${#corpus_rows[@]} -eq 5 ]] || { echo "expected five physical corpora" >&2; exit 2; }

cp "$matrix_json" "$result_root/matrix.json"
cp "$input_root/workloads.json" "$result_root/workloads.json"
cp "$input_root/direct_requests.json" "$result_root/direct_requests.json"
cp "$input_root/template.sha256" "$result_root/template.sha256"
cp "$input_root/working-set-uniqueness.csv" "$result_root/working-set-uniqueness.csv"
cp "$0" "$result_root/prepare_figure9_16_systems.sh"
cp "$nodes_env" "$result_root/nodes.env"
git -C "$workspace_dir/.dist/vhive-figure9-16" rev-parse HEAD >"$result_root/vhive.commit.txt"
sha256sum "$workspace_dir/.dist/vhive-figure9-16/bin/relay-figure9-16-direct-all" \
  "$workspace_dir/.dist/vhive-figure9-16/bin/snapshot-converter-figure9-16" >"$result_root/binaries.sha256"
ssh "${ssh_opts[@]}" "$worker" "tc qdisc show dev ifb0" >"$result_root/platform/worker-ifb.start.txt"
grep -Eq '^qdisc tbf .*rate 10Gbit .*lat 400ms' "$result_root/platform/worker-ifb.start.txt"
ssh "${ssh_opts[@]}" "$backend" "tc qdisc show dev '$NODE_PRIVATE_INTERFACE'; df -h '$backend_mount'" \
  >"$result_root/platform/backend.start.txt"
! grep -q '^qdisc tbf ' "$result_root/platform/backend.start.txt"

index=0
for row in "${corpus_rows[@]}"; do
  IFS=$'\t' read -r corpus_id security chunk_size ws_coalescing <<<"$row"
  port=$((port_base + index))
  container=snapshare-fig916-${corpus_id//-/_}-$stamp
  data_dir=$backend_mount/figure9-16/corpus-$corpus_id-$stamp
  container=${container//_/-}
  if awk -F, -v c="$corpus_id" 'NR>1 && $1==c {found=1} END {exit !found}' "$systems_csv"; then
    log "resume: keep completed corpus=$corpus_id"
    index=$((index + 1))
    continue
  fi
  if [[ $resume_systems == 1 ]] && ssh "${ssh_opts[@]}" "$backend" \
    "test \"\$(sudo docker inspect -f '{{.State.Running}} {{range .Mounts}}{{if eq .Destination \"/data\"}}{{.Source}}{{end}}{{end}}' '$container')\" = 'true $data_dir'"; then
    log "resume: verify completed but uncheckpointed corpus=$corpus_id"
    wait_minio "$container"
    verify_converter_log "$result_root/converters/$corpus_id.raw.log"
    verify_converter_log "$result_root/converters/$corpus_id.zstd3.log"
    [[ $(wc -l <"$result_root/inventory/$corpus_id.recipe.raw.sha256") -eq $((expected_revisions + 1)) ]]
    [[ $(wc -l <"$result_root/inventory/$corpus_id.recipe.zstd3.sha256") -eq $((expected_revisions + 1)) ]]
    cmp -s "$result_root/inventory/$corpus_id.recipe.raw.sha256" \
      "$result_root/inventory/$corpus_id.recipe.zstd3.sha256"
    capture_inventory_summary "$container" "$result_root/inventory/$corpus_id.txt"
    printf '%s,%s,%s,%s,%s,%s,%s\n' "$corpus_id" "$port" "$container" "$data_dir" \
      "$security" "$chunk_size" "$ws_coalescing" >>"$systems_csv"
    index=$((index + 1))
    continue
  fi
  log "prepare corpus=$corpus_id security=$security chunk=$chunk_size ws_coalescing=$ws_coalescing port=$port"
  start_corpus "$corpus_id" "$port" "$container" "$data_dir"
  mirror_template "$port" 2>&1 | tee "$result_root/converters/$corpus_id.mirror.log"
  run_converter "$corpus_id" "$port" "$security" "$chunk_size" "$ws_coalescing" raw \
    2>&1 | tee "$result_root/converters/$corpus_id.raw.log"
  verify_converter_log "$result_root/converters/$corpus_id.raw.log"
  capture_recipe_hashes "$container" "$result_root/inventory/$corpus_id.recipe.raw.sha256"
  run_converter "$corpus_id" "$port" "$security" "$chunk_size" "$ws_coalescing" zstd3 \
    2>&1 | tee "$result_root/converters/$corpus_id.zstd3.log"
  verify_converter_log "$result_root/converters/$corpus_id.zstd3.log"
  capture_recipe_hashes "$container" "$result_root/inventory/$corpus_id.recipe.zstd3.sha256"
  cmp -s "$result_root/inventory/$corpus_id.recipe.raw.sha256" \
    "$result_root/inventory/$corpus_id.recipe.zstd3.sha256" \
    || { echo "Zstd conversion changed recipe: $corpus_id" >&2; exit 5; }
  capture_inventory_summary "$container" "$result_root/inventory/$corpus_id.txt"
  printf '%s,%s,%s,%s,%s,%s,%s\n' "$corpus_id" "$port" "$container" "$data_dir" \
    "$security" "$chunk_size" "$ws_coalescing" >>"$systems_csv"
  index=$((index + 1))
done

[[ $(( $(wc -l <"$systems_csv") - 1 )) -eq 5 ]]
date -Is >"$result_root/SYSTEMS_COMPLETE"
log "FIGURE_9_16_SYSTEMS_COMPLETE corpora=5 runtime_configs=12 revisions=$expected_revisions port_base=$port_base"
