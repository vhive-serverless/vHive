#!/usr/bin/env bash
# Materialize the six Figure 11--13 systems for the current-base 17-workload
# input. Each VM-memory tier remains in an isolated MinIO namespace; the final
# workload manifest selects the matching endpoint for every profile/system.
set -euo pipefail

script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
workspace_dir=$(cd -- "$script_dir/.." && pwd)
nodes_env=${NODES_ENV:-$workspace_dir/colocation-infra/cluster/nodes.20260830.figure9-16-c6620-3node.env}
# shellcheck source=/dev/null
source "$nodes_env"

stamp=${STAMP:-20260902-currentbase17-r1}
input_root=${INPUT_ROOT:-$script_dir/results/figure9-16/${stamp}_c6620_inputs_tiered_r1}
result_root=${RESULT_ROOT:-$script_dir/results/figure9-16/${stamp}_c6620_systems_tiered_r1}
source_matrix=${FIGURE_MATRIX_JSON:-$script_dir/configs/figure9_16/matrix.json}
canonical_workloads=${CANONICAL_WORKLOADS_JSON:-$script_dir/configs/figure9_16/workloads.direct-all-r7.json}
canonical_requests=${CANONICAL_DIRECT_REQUESTS_JSON:-$script_dir/configs/figure9_16/direct_requests.json}
resume=${RESUME:-0}
backend=$BACKEND_NODE
ssh_opts=(-A -oStrictHostKeyChecking=no -oServerAliveInterval=20 -oServerAliveCountMax=6)

die() { printf 'ERROR: %s\n' "$*" >&2; exit 1; }
log() { printf '%s %s\n' "$(date -Is)" "$*" | tee -a "$result_root/launcher.log"; }

[[ ${AUTHORIZED_EXPERIMENT_ID:-} == figure9-16-direct-er020-er069-er032 ]] || die "wrong experiment"
[[ $resume == 0 || $resume == 1 ]] || die "RESUME must be 0 or 1"
[[ -s $input_root/TIERED_INPUTS_COMPLETE && -s $source_matrix \
   && -s $canonical_workloads && -s $canonical_requests ]] || die "tiered input is incomplete"
[[ $(jq '.workloads | length' "$input_root/workloads.json") == 17 ]] || die "expected 17 workloads"
if [[ -e $result_root ]]; then
  [[ $resume == 1 && ! -e $result_root/ORACLE_CORPUS_COMPLETE ]] || die "result root exists or is complete"
else
  mkdir -p "$result_root"
fi
mkdir -p "$result_root/provenance"
cp "$0" "$result_root/prepare_figure9_16_current_base_tiered_systems.sh"
cp "$nodes_env" "$result_root/nodes.env"
sha256sum "$input_root/workloads.json" "$input_root/direct_requests.json" \
  "$input_root/working-set-uniqueness.csv" "$source_matrix" >"$result_root/provenance/inputs.sha256"

run_tier() {
  local vm_mib=$1 expected=$2 input_port=$3 port_base=$4 oracle_port=$5
  local tier_input=$input_root/tier-$vm_mib
  local tier_systems=$result_root/tier-$vm_mib/systems
  local tier_oracle=$result_root/tier-$vm_mib/oracle
  local tier_matrix=$result_root/tier-$vm_mib/matrix.json
  local tier_stamp=${stamp}-tier${vm_mib}
  local suffix=tier${vm_mib}
  local source_id=full-dedup-4k-$suffix
  local reference_id=partial-4k-$suffix
  local source_container=snapshare-fig916-$source_id-$tier_stamp
  local reference_container=snapshare-fig916-$reference_id-$tier_stamp
  mkdir -p "$result_root/tier-$vm_mib"
  if [[ ! -s $tier_matrix ]]; then
    jq --arg suffix "-$suffix" '.systems |= map(.corpus_id += $suffix)' \
      "$source_matrix" >"$tier_matrix"
  fi
  [[ $(jq '[.systems[].corpus_id] | unique | length' "$tier_matrix") == 5 ]] \
    || die "tier matrix corpus count failed vm=$vm_mib"
  if [[ ! -s $tier_systems/SYSTEMS_COMPLETE ]]; then
    local resume_systems=0
    [[ ! -e $tier_systems ]] || resume_systems=1
    log "begin five physical corpora vm_mib=$vm_mib revisions=$expected ports=$port_base-$((port_base+4))"
    STAMP=$tier_stamp NODES_ENV=$nodes_env INPUT_ROOT=$tier_input RESULT_ROOT=$tier_systems \
      EXPECTED_REVISIONS=$expected RESUME_SYSTEMS=$resume_systems CORPUS_PORT_BASE=$port_base \
      FIGURE_MATRIX_JSON=$tier_matrix TEMPLATE_CONTAINER=snapshare-fig916-template-${stamp}-${vm_mib} \
      "$script_dir/prepare_figure9_16_systems.sh"
  else
    log "resume: five physical corpora complete vm_mib=$vm_mib"
  fi
  [[ $(awk -F, 'NR>1 {n++} END {print n+0}' "$tier_systems/corpora.csv") == 5 ]] \
    || die "five-corpus gate failed vm=$vm_mib"
  if [[ ! -s $tier_oracle/ORACLE_CORPUS_COMPLETE ]]; then
    [[ ! -e $tier_oracle ]] || die "incomplete oracle root requires explicit inspection: $tier_oracle"
    log "begin Full Dedup oracle vm_mib=$vm_mib revisions=$expected port=$oracle_port"
    MODE=materialize STAMP=${tier_stamp}-oracle NODES_ENV=$nodes_env \
      EXPECTED_REVISIONS=$expected RESULT_ROOT=$tier_oracle \
      WORKLOADS_JSON=$tier_input/workloads.json DIRECT_REQUESTS_JSON=$tier_input/direct_requests.json \
      MATRIX_JSON=$tier_matrix SYSTEMS_ROOT=$tier_systems \
      FULL_DEDUP_SOURCE_CONTAINER=$source_container FULL_DEDUP_SOURCE_PORT=$((port_base+2)) \
      SPLITSNAP_REFERENCE_CONTAINER=$reference_container SPLITSNAP_REFERENCE_PORT=$((port_base+4)) \
      FULL_DEDUP_ORACLE_CONTAINER=snapshare-fig916-full-dedup-oracle-$suffix-$stamp \
      FULL_DEDUP_ORACLE_PORT=$oracle_port FULL_DEDUP_ORACLE_CORPUS_ID=full-dedup-oracle-$suffix \
      FULL_DEDUP_ORACLE_DATA_DIR=/mnt/snapshare-zstd-streaming/figure9-16/corpus-full-dedup-oracle-$suffix-$stamp \
      "$script_dir/prepare_figure9_16_full_dedup_oracle.sh"
  else
    log "resume: Full Dedup oracle complete vm_mib=$vm_mib"
  fi
}

# Input is X0, five physical corpora are X1--X5, and the oracle is X6.
run_tier 2048 3 9470 9471 9476
run_tier 3072 1 9480 9481 9486
run_tier 512 13 9460 9461 9466

# Tier construction order is 2 GiB, 3 GiB, then 512 MiB, but all paper plots
# and the Figure 4 old13/new4 split require the canonical old-13 + new-4 order.
# Reorder by profile while preserving every generated snapshot/tier field.
jq -n --slurpfile generated "$input_root/workloads.json" \
  --slurpfile canonical "$canonical_workloads" '
  $generated[0] as $g |
  {schema_version:($g.schema_version // 1),
   workloads:[
     $canonical[0].workloads[].profile as $profile |
     $g.workloads[] | select(.profile == $profile)
   ]}
' >"$result_root/workloads.base.json"
jq -n --slurpfile generated "$input_root/direct_requests.json" \
  --slurpfile canonical "$canonical_requests" '
  $generated[0] as $g |
  $g + {requests:[
    $canonical[0].requests[].profile as $profile |
    $g.requests[] | select(.profile == $profile)
  ]}
' >"$result_root/direct_requests.json"
diff -u <(jq -r '.workloads[].profile' "$canonical_workloads") \
  <(jq -r '.workloads[].profile' "$result_root/workloads.base.json") >/dev/null \
  || die "canonical workload order gate failed"
diff -u <(jq -r '.requests[].profile' "$canonical_requests") \
  <(jq -r '.requests[].profile' "$result_root/direct_requests.json") >/dev/null \
  || die "canonical request order gate failed"
cp "$input_root/working-set-uniqueness.csv" "$result_root/working-set-uniqueness.csv"
cp "$result_root/tier-512/oracle/matrix.paper-oracle.json" "$result_root/matrix.json"
printf '%s\n' 'corpus_id,port,container,data_dir,security,chunk_size,ws_coalescing' >"$result_root/corpora.csv"
for vm_mib in 512 2048 3072; do
  tail -n +2 "$result_root/tier-$vm_mib/oracle/corpora.paper-oracle.csv" >>"$result_root/corpora.csv"
done
[[ $(awk -F, 'NR>1 {n++; ids[$1]++} END {bad=0; for (id in ids) if (ids[id]!=1) bad=1; print n ":" bad}' \
    "$result_root/corpora.csv") == 18:0 ]] || die "combined corpus IDs are missing or duplicated"

jq -n \
  --slurpfile inputs "$input_root/workloads.json" \
  --slurpfile matrix512 "$result_root/tier-512/oracle/matrix.paper-oracle.json" \
  --slurpfile matrix2048 "$result_root/tier-2048/oracle/matrix.paper-oracle.json" \
  --slurpfile matrix3072 "$result_root/tier-3072/oracle/matrix.paper-oracle.json" \
  --rawfile corpora512 "$result_root/tier-512/oracle/corpora.paper-oracle.csv" \
  --rawfile corpora2048 "$result_root/tier-2048/oracle/corpora.paper-oracle.csv" \
  --rawfile corpora3072 "$result_root/tier-3072/oracle/corpora.paper-oracle.csv" '
  def corpus_row($csv; $id):
    ($csv | split("\n") | .[1:] | map(select(length>0) | split(",")) |
      map(select(.[0] == $id)) | .[0]) as $r |
    {corpus_id:$r[0], port:($r[1]|tonumber), container:$r[2]};
  def overrides($matrix; $csv):
    reduce $matrix.systems[] as $s ({}; .[$s.id] = corpus_row($csv; $s.corpus_id));
  (overrides($matrix512[0]; $corpora512)) as $o512 |
  (overrides($matrix2048[0]; $corpora2048)) as $o2048 |
  (overrides($matrix3072[0]; $corpora3072)) as $o3072 |
  $inputs[0] |
  .workloads |= map(. + {corpus_overrides:
    (if .vm_mib == 512 then $o512 elif .vm_mib == 2048 then $o2048 else $o3072 end)})
' >"$result_root/workloads.json"

jq -e '
  (.workloads | length) == 17 and
  ([.workloads[].profile] | unique | length) == 17 and
  ([.workloads[].snapshot] | unique | length) == 17 and
  all(.workloads[]; (.vm_mib == 512 or .vm_mib == 2048 or .vm_mib == 3072) and
    (.corpus_overrides | keys | length) == 6)
' "$result_root/workloads.json" >/dev/null || die "combined workload endpoint gate failed"

: >"$result_root/provenance/endpoint-audit.tsv"
while IFS=$'\t' read -r profile snapshot system port container; do
  ssh "${ssh_opts[@]}" "$backend" \
    "test \"\$(sudo docker inspect -f '{{.State.Running}}' '$container')\" = true; sudo docker exec '$container' sh -lc 'mc alias set eval http://127.0.0.1:9000 minio minio123 >/dev/null; mc stat eval/snapshots/$snapshot/recipe_file >/dev/null'" \
    </dev/null
  printf '%s\t%s\t%s\t%s\t%s\n' "$profile" "$snapshot" "$system" "$port" "$container" \
    >>"$result_root/provenance/endpoint-audit.tsv"
done < <(jq -r '.workloads[] as $w | $w.corpus_overrides | to_entries[] |
  [$w.profile,$w.snapshot,.key,.value.port,.value.container] | @tsv' "$result_root/workloads.json")
[[ $(wc -l <"$result_root/provenance/endpoint-audit.tsv") == 102 ]] || die "endpoint audit count failed"

sha256sum "$result_root"/tier-*/oracle/{matrix.paper-oracle.json,corpora.paper-oracle.csv,workloads.json} \
  "$canonical_workloads" "$canonical_requests" \
  "$result_root/matrix.json" "$result_root/workloads.json" "$result_root/direct_requests.json" \
  "$result_root/corpora.csv" \
  >"$result_root/provenance/outputs.sha256"
date -Is >"$result_root/SYSTEMS_COMPLETE"
date -Is >"$result_root/ORACLE_CORPUS_COMPLETE"
log "TIERED_CURRENT_BASE_SYSTEMS_COMPLETE workloads=17 corpora=18 endpoints=102 result=$result_root"
