#!/usr/bin/env bash
# Rebuild all 17 single-function inputs from current-compatible base snapshots.
# Each VM-memory tier uses an isolated MinIO namespace because vHive stores its
# base under the fixed `base` revision name and the Firecracker memory layout is
# part of the snapshot contract.
set -euo pipefail

script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
workspace_dir=$(cd -- "$script_dir/.." && pwd)
nodes_env=${NODES_ENV:-$workspace_dir/colocation-infra/cluster/nodes.20260830.figure9-16-c6620-3node.env}
# shellcheck source=/dev/null
source "$nodes_env"

stamp=${STAMP:-20260902-currentbase17-r1}
result_root=${RESULT_ROOT:-$script_dir/results/figure9-16/${stamp}_c6620_inputs_tiered_r1}
source_workloads=${SOURCE_WORKLOADS_JSON:-$script_dir/configs/figure9_16/workloads.direct-all-r7.json}
source_requests=${SOURCE_DIRECT_REQUESTS_JSON:-$script_dir/configs/figure9_16/direct_requests.json}
relay_binary=${FIGURE_RELAY_BINARY:-/users/Liquidz/vhive-snapshare/bin/relay-figure4-snapshot-alignment}
local_relay_binary=${FIGURE_LOCAL_RELAY_BINARY:-$workspace_dir/.dist/vhive-figure4-snapshot-alignment/bin/relay-figure4-snapshot-alignment}
vhive_source_dir=${FIGURE_VHIVE_SOURCE_DIR:-$workspace_dir/.dist/vhive-figure4-snapshot-alignment}
resume=${RESUME:-0}

die() { printf 'ERROR: %s\n' "$*" >&2; exit 1; }
[[ ${AUTHORIZED_EXPERIMENT_ID:-} == figure9-16-direct-er020-er069-er032 ]] || die "wrong experiment"
[[ $resume == 0 || $resume == 1 ]] || die "RESUME must be 0 or 1"
[[ $stamp =~ ^[0-9]{8}[a-z0-9-]*$ ]] || die "invalid stamp"
[[ -s $source_workloads && -s $source_requests && -x $local_relay_binary ]] || die "frozen input or relay missing"
[[ $(jq '.workloads | length' "$source_workloads") == 17 ]] || die "expected 17 source workloads"
[[ $(jq '.requests | length' "$source_requests") == 17 ]] || die "expected 17 direct requests"
[[ $(jq '[.workloads[] | select(.vm_mib==512)] | length' "$source_workloads") == 13 ]] || die "512-MiB count mismatch"
[[ $(jq '[.workloads[] | select(.vm_mib==2048)] | length' "$source_workloads") == 3 ]] || die "2-GiB count mismatch"
[[ $(jq '[.workloads[] | select(.vm_mib==3072)] | length' "$source_workloads") == 1 ]] || die "3-GiB count mismatch"

if [[ -e $result_root ]]; then
  [[ $resume == 1 && ! -e $result_root/TIERED_INPUTS_COMPLETE ]] || die "result root exists or is already complete"
else
  mkdir -p "$result_root"
fi
launcher=$result_root/launcher.log
log() { printf '%s %s\n' "$(date -Is)" "$*" | tee -a "$launcher"; }

tmp_dir=$(mktemp -d)
trap 'rm -rf -- "$tmp_dir"' EXIT

make_tier_manifests() {
  local vm_mib=$1 workloads_out=$2 requests_out=$3 expected=$4
  jq --argjson vm "$vm_mib" --arg stamp "$stamp" '
    .workloads |= [
      .[] | select(.vm_mib == $vm) |
      .source = "regenerate-direct" |
      del(.corpus_overrides) |
      .fresh_source_delay_seconds =
        (if .profile == "video-analytics-standalone-python-1" then 30 else 10 end) |
      .snapshot =
        (if .profile == "aes-go-45000-45450" then
           "cold-aes-go-45000-45450-direct512-currentbase-ws5-" + $stamp + "-0"
         else
           "snapshare-" + .profile + "-direct" + ($vm|tostring) + "-currentbase-ws5-" + $stamp
         end)
    ]
  ' "$source_workloads" >"$workloads_out"
  jq --slurpfile workloads "$workloads_out" '
    .requests = [.requests[] as $request |
      select(any($workloads[0].workloads[]; .profile == $request.profile)) |
      $request]
  ' "$source_requests" >"$requests_out"
  [[ $(jq '.workloads | length' "$workloads_out") == "$expected" ]] || die "tier workload count mismatch vm=$vm_mib"
  [[ $(jq '.requests | length' "$requests_out") == "$expected" ]] || die "tier request count mismatch vm=$vm_mib"
  jq -e --argjson vm "$vm_mib" '
    ([.workloads[].profile] | unique | length) == (.workloads | length) and
    ([.workloads[].snapshot] | unique | length) == (.workloads | length) and
    all(.workloads[]; .vm_mib == $vm and .source == "regenerate-direct" and
      (.fresh_source_delay_seconds == 10 or .fresh_source_delay_seconds == 30) and
      .request_path == "direct" and (.relay_args | length) == 0)
  ' "$workloads_out" >/dev/null || die "tier manifest gate failed vm=$vm_mib"
}

run_tier() {
  local vm_mib=$1 port=$2 expected=$3 tier_stamp tier_root workloads_json requests_json resume_inputs=0
  tier_stamp=${stamp}-${vm_mib}
  tier_root=$result_root/tier-$vm_mib
  workloads_json=$tmp_dir/workloads.$vm_mib.json
  requests_json=$tmp_dir/direct-requests.$vm_mib.json
  make_tier_manifests "$vm_mib" "$workloads_json" "$requests_json" "$expected"
  if [[ -s $tier_root/INPUTS_COMPLETE ]]; then
    log "resume: tier complete vm_mib=$vm_mib result=$tier_root"
    return
  fi
  [[ ! -e $tier_root ]] || resume_inputs=1
  log "begin current-base tier vm_mib=$vm_mib workloads=$expected port=$port resume=$resume_inputs"
  STAMP=$tier_stamp NODES_ENV=$nodes_env RESULT_ROOT=$tier_root RESUME_INPUTS=$resume_inputs \
    FIGURE_WORKLOADS_JSON=$workloads_json FIGURE_DIRECT_REQUESTS_JSON=$requests_json \
    TEMPLATE_PORT=$port FIGURE_RELAY_BINARY=$relay_binary \
    FIGURE_LOCAL_RELAY_BINARY=$local_relay_binary FIGURE_VHIVE_SOURCE_DIR=$vhive_source_dir \
    RESTORE_UNION_BASE_SNAP=1 RESTORE_UNION_BASE_SOURCE=current \
    RESTORE_UNION_PRE_CLEAN_LOCAL=1 RESTORE_UNION_CLEAN_LOCAL=0 RESTORE_UNION_REUSE_LOCAL=1 \
    "$script_dir/prepare_figure9_16_inputs.sh"
  [[ $(awk -F, 'NR>1 {n++; if ($2!=$3) bad=1} END {print n ":" bad+0}' \
      "$tier_root/working-set-uniqueness.csv") == "$expected:0" ]] || die "WS uniqueness failed vm=$vm_mib"
  grep -Fxq "vm_mib=$vm_mib" "$tier_root/current-base-size.txt" || die "base memory tier mismatch vm=$vm_mib"
  log "current-base tier complete vm_mib=$vm_mib workloads=$expected"
}

printf '%s\n' 'vm_mib,input_port,expected_workloads,result' >"$result_root/tiers.csv"
printf '512,9460,13,tier-512\n2048,9470,3,tier-2048\n3072,9480,1,tier-3072\n' >>"$result_root/tiers.csv"
cp "$0" "$result_root/prepare_figure9_16_current_base_tiers.sh"
cp "$nodes_env" "$result_root/nodes.env"
sha256sum "$source_workloads" "$source_requests" "$local_relay_binary" \
  >"$result_root/frozen-inputs.sha256"

run_tier 2048 9470 3
run_tier 3072 9480 1
run_tier 512 9460 13

jq -n \
  --slurpfile generated <(jq -s '{schema_version:1, workloads:(map(.workloads[]))}' \
    "$result_root"/tier-*/workloads.json) \
  --slurpfile canonical "$source_workloads" '
  $generated[0] as $g |
  {schema_version:($g.schema_version // 1),
   workloads:[
     $canonical[0].workloads[].profile as $profile |
     $g.workloads[] | select(.profile == $profile)
   ]}
' >"$result_root/workloads.json"
jq -n \
  --slurpfile generated <(jq -s '.[0] as $base | $base + {requests:(map(.requests[]))}' \
    "$result_root"/tier-*/direct_requests.json) \
  --slurpfile canonical "$source_requests" '
  $generated[0] as $g |
  $g + {requests:[
    $canonical[0].requests[].profile as $profile |
    $g.requests[] | select(.profile == $profile)
  ]}
' >"$result_root/direct_requests.json"
diff -u <(jq -r '.workloads[].profile' "$source_workloads") \
  <(jq -r '.workloads[].profile' "$result_root/workloads.json") >/dev/null \
  || die "canonical workload order gate failed"
diff -u <(jq -r '.requests[].profile' "$source_requests") \
  <(jq -r '.requests[].profile' "$result_root/direct_requests.json") >/dev/null \
  || die "canonical request order gate failed"
{
  printf '%s\n' 'snapshot,rows,unique_pfns,vm_mib'
  for vm_mib in 512 2048 3072; do
    awk -F, -v vm="$vm_mib" 'NR>1 {print $1 "," $2 "," $3 "," vm}' \
      "$result_root/tier-$vm_mib/working-set-uniqueness.csv"
  done
} >"$result_root/working-set-uniqueness.csv"

[[ $(jq '.workloads | length' "$result_root/workloads.json") == 17 ]] || die "combined workload count failed"
[[ $(jq '.requests | length' "$result_root/direct_requests.json") == 17 ]] || die "combined request count failed"
[[ $(awk -F, 'NR>1 {n++; if ($2!=$3) bad=1} END {print n ":" bad+0}' \
    "$result_root/working-set-uniqueness.csv") == 17:0 ]] || die "combined WS uniqueness failed"
date -Is >"$result_root/TIERED_INPUTS_COMPLETE"
log "TIERED_CURRENT_BASE_INPUTS_COMPLETE workloads=17 tiers=3 result=$result_root"
