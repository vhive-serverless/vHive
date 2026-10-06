#!/usr/bin/env bash
# Prepare the paper-eligible Full Dedup baseline for all 17 semantic revisions.
#
# MODE=validate (default) is read-only: it validates the global raw-content
# canonical store against every accepted SplitSnap transfer view and writes the
# exact copy/runtime plan. MODE=materialize additionally creates an isolated
# MinIO corpus containing the canonical Full Dedup store plus those transient,
# pre-measurement SplitSnap views. Existing corpora and services are never
# modified or stopped.
set -euo pipefail

script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
workspace_dir=$(cd -- "$script_dir/.." && pwd)
nodes_env=${NODES_ENV:-$workspace_dir/colocation-infra/cluster/nodes.20260830.figure9-16-c6620-3node.env}
# shellcheck source=/dev/null
source "$nodes_env"

mode=${MODE:-validate}
expected_revisions=${EXPECTED_REVISIONS:-17}
stamp=${STAMP:-20260830_c6620_full_dedup_oracle_direct17_r2}
result_root=${RESULT_ROOT:-$script_dir/results/full-dedup/$stamp}
workloads=${WORKLOADS_JSON:-$script_dir/configs/figure9_16/workloads.json}
direct_requests=${DIRECT_REQUESTS_JSON:-$script_dir/configs/figure9_16/direct_requests.json}
matrix=${MATRIX_JSON:-$script_dir/configs/figure9_16/matrix.json}
systems_root=${SYSTEMS_ROOT:-$script_dir/results/figure9-16/20260830-direct-r2_c6620_systems_r1}
worktree=$workspace_dir/.dist/vhive-full-dedup-oracle
binary=$worktree/bin/materialize-full-dedup-oracle-ws
worker=${WORKER_NODES[0]}
backend=$BACKEND_NODE
backend_ip=$BACKEND_PRIVATE_IP
source_container=${FULL_DEDUP_SOURCE_CONTAINER:-snapshare-fig916-full-dedup-4k-20260830-direct-r2}
reference_container=${SPLITSNAP_REFERENCE_CONTAINER:-snapshare-fig916-partial-4k-20260830-direct-r2}
source_port=${FULL_DEDUP_SOURCE_PORT:-9412}
reference_port=${SPLITSNAP_REFERENCE_PORT:-9414}
oracle_container=${FULL_DEDUP_ORACLE_CONTAINER:-snapshare-fig916-full-dedup-splitsnap-oracle-20260830-direct-r2}
oracle_port=${FULL_DEDUP_ORACLE_PORT:-9415}
oracle_corpus_id=${FULL_DEDUP_ORACLE_CORPUS_ID:-full-dedup-splitsnap-oracle}
backend_mount=${BACKEND_MOUNT:-/mnt/snapshare-zstd-streaming}
oracle_dir=${FULL_DEDUP_ORACLE_DATA_DIR:-$backend_mount/figure9-16/corpus-full-dedup-splitsnap-oracle-20260830-direct-r2}
verification_scope=${VERIFY_CANONICAL:-working-set}
remote_root=/users/Liquidz/figure9-16/full-dedup-oracle/$stamp
remote_binary=$remote_root/materialize-full-dedup-oracle-ws
backend_copy_plan=/users/Liquidz/full-dedup-oracle-$stamp.copy-plan.tsv
ssh_opts=(-A -oStrictHostKeyChecking=no -oServerAliveInterval=20 -oServerAliveCountMax=6)
launcher=$result_root/launcher.log

die() { printf 'ERROR: %s\n' "$*" >&2; exit 1; }
log() { printf '%s %s\n' "$(date -Is)" "$*" | tee -a "$launcher"; }

[[ $mode == validate || $mode == materialize ]] || die "MODE must be validate or materialize"
[[ $expected_revisions =~ ^[1-9][0-9]*$ ]] \
  || die "EXPECTED_REVISIONS must be at least one"
[[ $verification_scope == working-set || $verification_scope == all ]] \
  || die "VERIFY_CANONICAL must be working-set or all"
[[ ${EXPERIMENT_ID:-} == figure9-16-direct-er020-er069-er032 ]] || die "unexpected experiment"
[[ $worker == "$AUTHORIZED_CONTROL_NODE" && $backend == "$AUTHORIZED_BACKEND_NODE" ]] \
  || die "unexpected node mapping"
[[ ${WORKER_PRIVATE_IPS[0]} == "$AUTHORIZED_CONTROL_PRIVATE_IP" && $backend_ip == "$AUTHORIZED_BACKEND_PRIVATE_IP" ]] \
  || die "unexpected private IP mapping"
[[ $stamp =~ ^[a-zA-Z0-9._-]+$ ]] || die "invalid stamp"
[[ $source_port =~ ^[1-9][0-9]*$ && $source_port -le 65535 ]] || die "invalid source port"
[[ $reference_port =~ ^[1-9][0-9]*$ && $reference_port -le 65535 ]] || die "invalid reference port"
[[ $source_port != "$reference_port" ]] || die "source and reference ports must differ"
[[ $oracle_port =~ ^[1-9][0-9]*$ && $oracle_port -le 65535 ]] || die "invalid oracle port"
[[ $oracle_corpus_id =~ ^[a-zA-Z0-9._-]+$ ]] || die "invalid oracle corpus id"
[[ $oracle_dir == "$backend_mount"/figure9-16/* ]] || die "invalid oracle data directory"
[[ -s $workloads && -s $direct_requests && -s $matrix && -s $systems_root/SYSTEMS_COMPLETE \
   && -s $systems_root/working-set-uniqueness.csv ]] || die "frozen inputs are incomplete"
[[ $(jq '.workloads | length' "$workloads") == "$expected_revisions" ]] || die "unexpected workload count"
[[ ! -e $result_root ]] || die "refusing existing result root: $result_root"
mkdir -p "$result_root"/{platform,provenance}
touch "$launcher"

verify_cpu() {
  ssh "${ssh_opts[@]}" "$1" 'bash -s -- verify-2.1ghz' \
    <"$script_dir/configure_c6620_cpu_baseline.sh" >"$2"
}

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

write_copy_plan() {
  local expected_shared expected_private
  {
    printf 'shared\tws_shared/base_rootfs/content\n'
    printf 'shared\tws_shared/base_rootfs/index\n'
    jq -r '[.workloads[].image_inventory] | unique[] | "shared\tws_shared/images/\(.)/content", "shared\tws_shared/images/\(.)/index"' "$result_root/workloads.json"
    jq -r '.workloads[].snapshot as $s | [
      "working_set_pages_content_private",
      "working_set_pages_content_private.zstd.frames",
      "working_set_pages_content_private.zstd.json",
      "working_set_pages_index_private"
    ][] | "revision\t\($s)/\(.)"' "$result_root/workloads.json"
  } >"$result_root/transient-copy-plan.tsv"
  expected_shared=$((2 + 2 * $(jq '[.workloads[].image_inventory] | unique | length' "$result_root/workloads.json")))
  expected_private=$((4 * expected_revisions))
  [[ $(awk -F '\t' '$1=="shared" {n++} END {print n+0}' "$result_root/transient-copy-plan.tsv") == "$expected_shared" ]] \
    || die "unexpected shared-view object count"
  [[ $(awk -F '\t' '$1=="revision" {n++} END {print n+0}' "$result_root/transient-copy-plan.tsv") == "$expected_private" ]] \
    || die "unexpected revision-private object count"
}

run_batch_validation() {
  local canonical_port=$1 reference_port_for_run=$2 remote_manifest=$3 destination=$4
  ssh "${ssh_opts[@]}" "$worker" "'$remote_binary' \
    -minioURL '$backend_ip:$canonical_port' -bucket snapshots \
    -batchWorkloads '$remote_manifest' \
    -referenceMinioURL '$backend_ip:$reference_port_for_run' \
    -verifyCanonical '$verification_scope' -workers 28 \
    -report '$remote_root/$destination'" \
    | tee "$result_root/${destination%.json}.stdout.json"
  scp "${ssh_opts[@]}" "$worker:$remote_root/$destination" "$result_root/$destination" >/dev/null
  cmp -s "$result_root/${destination%.json}.stdout.json" "$result_root/$destination" \
    || die "stdout and persisted batch reports differ"
  jq -e --arg scope "$verification_scope" --argjson expected "$expected_revisions" '
    .semantic_revisions == $expected and .verification_scope == $scope and
    .canonical_page_objects > 0 and .working_set_unique_hashes > 0 and
    (if $scope == "working-set" then
       .verified_unique_page_objects == .working_set_unique_hashes
     else
       .verified_unique_page_objects == .canonical_page_objects
     end) and
    ([.revisions[] |
      select(.working_set_pages != .covered_working_set_pages or
             .private_raw_bytes <= 0 or .private_compressed_bytes <= 0 or
             .private_compressed_frames <= 0)] | length) == 0
  ' "$result_root/$destination" >/dev/null || die "batch Full Dedup gate failed"
}

generate_runtime_overrides() {
  jq --arg oracle_corpus_id "$oracle_corpus_id" '
    .full_dedup_paper_protocol = "SplitSnap restore/WS path backed by global raw-content persistent dedup; accepted transfer views are validated and materialized before measurement with assembly time/temporary footprint excluded" |
    .systems |= map(
      if .id == "full-dedup-raw" then
        .corpus_id = $oracle_corpus_id |
        .paper_name = "Full Dedup" | .paper_eligible = true |
        .security = "partial" | .ws_coalescing = true | .base_snap = true |
        .ws_compression = false | .chunk_compression = false |
        .oracle_boundary = "zero-cost pre-measurement SplitSnap-format transfer view"
      elif .id == "full-dedup-zstd3" then
        .corpus_id = $oracle_corpus_id |
        .paper_name = "Full Dedup + Zstd-3" | .paper_eligible = true |
        .security = "partial" | .ws_coalescing = true | .base_snap = true |
        .ws_compression = true | .chunk_compression = true |
        .oracle_boundary = "zero-cost pre-measurement SplitSnap-format transfer view"
      else . end
    )
  ' "$matrix" >"$result_root/matrix.paper-oracle-all.json"
  jq '
    .systems = [
      (.systems[] | select(.id == "chunks-128k-zstd3") | .paper_name = "Chunks"),
      (.systems[] | select(.id == "pages-4k-zstd3") | .paper_name = "Pages"),
      (.systems[] | select(.id == "ws-zstd3") | .paper_name = "WS"),
      (.systems[] | select(.id == "no-image-zstd3") | .paper_name = "No image sharing"),
      (.systems[] | select(.id == "splitsnap-zstd3") | .paper_name = "SplitSnap"),
      (.systems[] | select(.id == "full-dedup-zstd3") | .paper_name = "Full Dedup")
    ] |
    .compression_protocol = "all six systems use Zstd-3; no Raw row is included"
  ' "$result_root/matrix.paper-oracle-all.json" >"$result_root/matrix.paper-oracle.json"
  cp "$systems_root/corpora.csv" "$result_root/corpora.paper-oracle.csv"
  printf '%s,%s,%s,%s,%s,%s,%s\n' \
    "$oracle_corpus_id" "$oracle_port" "$oracle_container" "$oracle_dir" partial 4096 true \
    >>"$result_root/corpora.paper-oracle.csv"
  [[ $(jq '[.systems[] | select(.paper_eligible == false)] | length' "$result_root/matrix.paper-oracle.json") == 0 ]] \
    || die "paper matrix still contains ineligible systems"
  [[ $(awk -F, -v id="$oracle_corpus_id" 'NR>1 && $1==id {n++} END {print n+0}' "$result_root/corpora.paper-oracle.csv") == 1 ]] \
    || die "oracle corpus row is missing or duplicated"
}

generate_runtime_workloads() {
  cp "$workloads" "$result_root/workloads.json"
  [[ $(jq '.workloads | length' "$result_root/workloads.json") == "$expected_revisions" ]] || die "runtime workload count changed"
  cmp -s "$workloads" "$result_root/workloads.json" || die "runtime workload manifest changed while freezing"
  jq -e --argjson expected "$expected_revisions" '
    (.workloads | length) == $expected and
    ([.workloads[].profile] | unique | length) == $expected and
    ([.workloads[].snapshot] | unique | length) == $expected and
    all(.workloads[]; (.vm_mib | type) == "number" and .vm_mib > 0)
  ' "$result_root/workloads.json" >/dev/null || die "runtime workload manifest gate failed"
  if jq -e '.workloads[] | select(.profile == "aes-go-45000-45450")' "$result_root/workloads.json" >/dev/null; then
    jq -e '
      [.workloads[] | select(.profile == "aes-go-45000-45450")] as $aes |
      ($aes | length) == 1 and ($aes[0].vm_mib == 512) and
      ($aes[0].snapshot | test("^cold-aes-go-45000-45450-direct512-(currentbase-)?ws5-[A-Za-z0-9._-]+-0$"))
    ' "$result_root/workloads.json" >/dev/null || die "regenerated direct512/ws5 AES snapshot was not frozen"
  fi
}

log "BEGIN mode=$mode revisions=$expected_revisions verification_scope=$verification_scope"
cp "$0" "$result_root/prepare_figure9_16_full_dedup_oracle.sh"
cp "$nodes_env" "$result_root/nodes.env"
cp "$workloads" "$result_root/workloads.input.json"
cp "$direct_requests" "$result_root/direct_requests.json"
cp "$matrix" "$result_root/matrix.input.json"
cp "$systems_root/working-set-uniqueness.csv" "$result_root/working-set-uniqueness.csv"
generate_runtime_workloads
git -C "$worktree" rev-parse HEAD >"$result_root/provenance/vhive.commit.txt"
git -C "$worktree" status --short --branch >"$result_root/provenance/vhive.status.txt"
go -C "$worktree" build -o "$binary" ./cmd/materialize_full_dedup_oracle_ws
sha256sum "$binary" >"$result_root/provenance/materializer.sha256"
verify_cpu "$worker" "$result_root/platform/worker.txt"
verify_cpu "$backend" "$result_root/platform/backend.txt"
ssh "${ssh_opts[@]}" "$backend" \
  "tc qdisc show dev '$NODE_PRIVATE_INTERFACE'; sudo docker inspect -f '{{.Name}} {{.State.Running}}' '$source_container' '$reference_container'" \
  >"$result_root/platform/backend-services.txt"
! grep -q '^qdisc tbf ' "$result_root/platform/backend-services.txt" || die "unexpected backend TBF"
ssh "${ssh_opts[@]}" "$worker" "tc qdisc show dev ifb0" >"$result_root/platform/worker-ifb.txt"
grep -Eq '^qdisc tbf .*rate 10Gbit .*lat 400ms' "$result_root/platform/worker-ifb.txt" \
  || die "worker ingress 10-Gbit gate failed"
grep -Fq "/$source_container true" "$result_root/platform/backend-services.txt" \
  || die "Full Dedup source corpus is not running"
grep -Fq "/$reference_container true" "$result_root/platform/backend-services.txt" \
  || die "SplitSnap reference corpus is not running"
write_copy_plan
ssh "${ssh_opts[@]}" "$worker" "test ! -e '$remote_root'; mkdir -p '$remote_root'"
scp "${ssh_opts[@]}" "$binary" "$worker:$remote_binary" >/dev/null
scp "${ssh_opts[@]}" "$workloads" "$worker:$remote_root/workloads.input.json" >/dev/null
scp "${ssh_opts[@]}" "$result_root/workloads.json" "$worker:$remote_root/workloads.json" >/dev/null
ssh "${ssh_opts[@]}" "$worker" "chmod 0755 '$remote_binary'"
run_batch_validation "$source_port" "$reference_port" "$remote_root/workloads.input.json" validation.source.json
generate_runtime_overrides

if [[ $mode == validate ]]; then
  touch "$result_root/PREPARE_COMPLETE"
  log "PREPARE_COMPLETE read_only=true revisions=$expected_revisions copy_objects=$(wc -l <"$result_root/transient-copy-plan.tsv")"
  exit 0
fi

log "materialize isolated oracle corpus container=$oracle_container port=$oracle_port"
ssh "${ssh_opts[@]}" "$backend" bash -s -- \
  "$source_container" "$reference_container" "$oracle_container" "$oracle_port" "$oracle_dir" "$backend_ip" <<'REMOTE'
set -euo pipefail
source_container=$1
reference_container=$2
oracle_container=$3
oracle_port=$4
oracle_dir=$5
backend_ip=$6
test "$(sudo docker inspect -f '{{.State.Running}}' "$source_container")" = true
test "$(sudo docker inspect -f '{{.State.Running}}' "$reference_container")" = true
test ! -e "$oracle_dir"
! sudo docker inspect "$oracle_container" >/dev/null 2>&1
test -z "$(ss -H -ltn "sport = :$oracle_port")"
sudo install -d -o 1000 -g 1000 "$oracle_dir"
sudo docker run -d --name "$oracle_container" -p "$backend_ip:$oracle_port:9000" \
  -e MINIO_ROOT_USER=minio -e MINIO_ROOT_PASSWORD=minio123 \
  -v "$oracle_dir:/data" quay.io/minio/minio:RELEASE.2024-12-18T13-15-44Z server /data
REMOTE
wait_minio "$oracle_container"
mc_exec "$oracle_container" 'mc mb eval/snapshots'

log "mirror immutable global Full Dedup corpus"
ssh "${ssh_opts[@]}" "$backend" bash -s -- "$source_container" "$backend_ip" "$oracle_port" <<'REMOTE'
set -euo pipefail
source_container=$1
backend_ip=$2
oracle_port=$3
sudo docker exec "$source_container" sh -lc '
  set -e
  mc alias set source http://127.0.0.1:9000 minio minio123 >/dev/null
  mc alias set oracle http://'"$backend_ip:$oracle_port"' minio minio123 >/dev/null
  mc mirror --quiet --retry --exclude "*/mem_file" --overwrite source/snapshots oracle/snapshots
'
REMOTE

log "copy validated transient SplitSnap view objects count=$(wc -l <"$result_root/transient-copy-plan.tsv")"
scp "${ssh_opts[@]}" "$result_root/transient-copy-plan.tsv" "$backend:$backend_copy_plan" >/dev/null
ssh "${ssh_opts[@]}" "$backend" bash -s -- \
  "$reference_container" "$oracle_container" "$backend_copy_plan" <<'REMOTE'
set -euo pipefail
reference_container=$1
oracle_container=$2
plan=$3
reference_ip=$(sudo docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' "$reference_container")
[[ $reference_ip =~ ^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$ ]]
sudo docker exec "$oracle_container" sh -lc \
  'mc alias set eval http://127.0.0.1:9000 minio minio123 >/dev/null; mc alias set reference http://'$reference_ip':9000 minio minio123 >/dev/null'
while IFS=$'\t' read -r kind object; do
  case $kind in shared|revision) ;; *) exit 2 ;; esac
  [[ $object =~ ^[A-Za-z0-9._/-]+$ ]] || exit 2
  sudo docker exec "$oracle_container" sh -lc \
    "mc cp --quiet reference/snapshots/$object eval/snapshots/$object"
done <"$plan"
REMOTE

run_batch_validation "$oracle_port" "$oracle_port" "$remote_root/workloads.json" validation.oracle.json
cp "$result_root/matrix.paper-oracle.json" "$result_root/matrix.json"
cp "$result_root/corpora.paper-oracle.csv" "$result_root/corpora.csv"
date -Is >"$result_root/SYSTEMS_COMPLETE"
date -Is >"$result_root/ORACLE_CORPUS_COMPLETE"
log "ORACLE_CORPUS_COMPLETE revisions=$expected_revisions container=$oracle_container port=$oracle_port corpus_id=$oracle_corpus_id formal_systems_root=$result_root"
