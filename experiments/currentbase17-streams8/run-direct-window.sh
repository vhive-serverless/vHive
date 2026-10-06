#!/usr/bin/env bash
# Derived from snapshare-eval/run_figure9_16_direct_window.sh; independent endpoint.
# One paper-aligned direct native window: 60 calls at absolute 1-RPS cadence.
# Run this on the dedicated loader. Every revision resolves to one of 60
# pre-materialized cold aliases after the relay removes the final two unique
# suffix components.
set -euo pipefail

if (( $# != 10 )); then
  echo "usage: $0 <relay-endpoint> <backend-ip> <output-dir> <calls> <interval-ms> <snapshot> <alias-tag> <profile> <workloads-json> <direct-requests-json>" >&2
  exit 2
fi

relay_endpoint=$1
backend_ip=$2
output_dir=$3
calls=$4
interval_ms=$5
snapshot=$6
alias_tag=$7
profile=$8
workloads_json=$9
direct_requests_json=${10}
direct_invoker=${FIGURE_DIRECT_INVOKER:-/users/Liquidz/figure9-16/bin/direct-invoker}

[[ $relay_endpoint =~ ^[0-9.]+:[0-9]+$ && $backend_ip =~ ^[0-9.]+$ ]] || exit 2
[[ $output_dir =~ ^/users/Liquidz/[a-zA-Z0-9_./-]+$ ]] || exit 2
[[ $calls == 60 && $interval_ms =~ ^[0-9]+$ && $interval_ms -ge 1000 ]] || { echo "requires 60 calls and interval >= 1000ms" >&2; exit 2; }
[[ $snapshot =~ ^[a-zA-Z0-9._-]+$ && $alias_tag =~ ^[a-zA-Z0-9._-]+$ ]] || exit 2
[[ $profile =~ ^[a-zA-Z0-9._-]+$ ]] || exit 2
[[ -x $direct_invoker && -s $workloads_json && -s $direct_requests_json ]] || exit 3
[[ ! -e $output_dir ]] || { echo "refusing existing output: $output_dir" >&2; exit 3; }
mkdir -p "$output_dir/calls"

entry=$(jq -ce --arg p "$profile" '.workloads[] | select(.profile==$p)' "$workloads_json")
request=$(jq -ce --arg p "$profile" '.requests[] | select(.profile==$p)' "$direct_requests_json")
[[ $(jq -r '.snapshot' <<<"$entry") == "$snapshot" ]] || exit 3
image=$(jq -r '.image' <<<"$entry")
function_args=$(jq -r '.function_args // ""' <<<"$entry")
function_env=$(jq -r '.function_env // ""' <<<"$entry")
function_port=$(jq -r '.function_port' <<<"$entry")
function_name=$(jq -r '.function_name' <<<"$request")
generator=$(jq -r '.generator' <<<"$request")
value=$(jq -r '.value // ""' <<<"$request")
function_method=$(jq -r '.function_method // "default"' <<<"$request")
reply_must_contain=$(jq -r '.reply_must_contain // ""' <<<"$request")
lower_bound=$(jq -r '.lower_bound // 1' <<<"$request")
upper_bound=$(jq -r '.upper_bound // 10' <<<"$request")
seed=$(jq -r '.seed // empty' <<<"$request")
[[ -n $seed ]] || seed=$(jq -r '.seed' "$direct_requests_json")
function_args=${function_args//__BACKEND_PRIVATE_IP__/$backend_ip}

[[ $image != null && $function_name != null ]] || exit 3
[[ $function_port =~ ^[1-9][0-9]*$ ]] || exit 3
[[ $generator =~ ^(fixed|unique|linear|random)$ ]] || exit 3
[[ $lower_bound =~ ^[0-9]+$ && $upper_bound =~ ^[0-9]+$ && $seed =~ ^-?[0-9]+$ ]] || exit 3
[[ $function_args != *'__BACKEND_PRIVATE_IP__'* ]] || exit 3

start_ns=$(date +%s%N)
nonce=$(date +%s)

invoke_one() {
  local index=$1 scheduled_ns=$2 index5 slot revision call_start_ns call_end_ns status
  index5=$(printf '%05d' "$index")
  slot=$((index - 1))
  revision=$snapshot-$alias_tag-$slot-$nonce-$index5
  call_start_ns=$(date +%s%N)
  set +e
  "$direct_invoker" \
    --address "$relay_endpoint" \
    --function-name "$function_name" \
    --image "$image" \
    --revision "$revision" \
    --args "$function_args" \
    --env "$function_env" \
    --function-port "$function_port" \
    --generator "$generator" \
    --value "$value" \
    --function-method "$function_method" \
    --lower-bound "$lower_bound" \
    --upper-bound "$upper_bound" \
    --seed "$seed" \
    --sequence-index "$slot" \
    --timeout 900s >"$output_dir/calls/$index5.out" 2>&1
  status=$?
  set -e
  call_end_ns=$(date +%s%N)
  printf '%s\t%s\t%s\t%s\t%s\t%s\t%s\n' \
    "$index" "$slot" "$revision" "$scheduled_ns" "$call_start_ns" "$call_end_ns" "$status" \
    >"$output_dir/calls/$index5.meta"
}

pids=()
for index in $(seq 1 "$calls"); do
  scheduled_ns=$((start_ns + (index - 1) * interval_ms * 1000000))
  now_ns=$(date +%s%N)
  if (( scheduled_ns > now_ns )); then
    delta_ns=$((scheduled_ns - now_ns))
    printf -v delay '%d.%09d' "$((delta_ns / 1000000000))" "$((delta_ns % 1000000000))"
    sleep "$delay"
  fi
  if (( interval_ms == 1000 )); then
    invoke_one "$index" "$scheduled_ns" &
    pids+=("$!")
  else
    # Low-rate isolation runs never overlap foreground invocations. The long
    # interval also allows asynchronous teardown; verify it from the raw logs.
    invoke_one "$index" "$scheduled_ns"
  fi
done

wait_status=0
for pid in "${pids[@]}"; do wait "$pid" || wait_status=1; done

printf 'index\tslot\trevision\tscheduled_ns\tstart_ns\tend_ns\te2e_ns\tstart_lag_ns\texit_status\treply_ok\tinput_bytes\treply_bytes\treply_sha256\tdirect_native\n' \
  >"$output_dir/invocations.tsv"
success=0
for index in $(seq 1 "$calls"); do
  index5=$(printf '%05d' "$index")
  IFS=$'\t' read -r meta_index slot revision scheduled_ns call_start_ns call_end_ns status \
    <"$output_dir/calls/$index5.meta"
  output=$output_dir/calls/$index5.out
  request_ns=$(sed -n 's/^DIRECT_REQUEST_NS=\([0-9][0-9]*\).*/\1/p' "$output" | tail -n 1)
  reply_bytes=$(sed -n 's/^DIRECT_REQUEST_NS=[0-9][0-9]* REPLY_BYTES=\([0-9][0-9]*\).*/\1/p' "$output" | tail -n 1)
  reply_sha=$(sed -n 's/^DIRECT_REQUEST_NS=[0-9][0-9]* REPLY_BYTES=[0-9][0-9]* REPLY_SHA256=\([0-9a-f][0-9a-f]*\).*/\1/p' "$output" | tail -n 1)
  [[ -n $request_ns ]] || request_ns=0
  [[ -n $reply_bytes ]] || reply_bytes=0
  [[ -n $reply_sha ]] || reply_sha=missing
  reply_ok=0
  if (( status == 0 && request_ns > 0 )) \
    && ! grep -Eq 'Error:|Failed|Server Error|Snapshot (Download|Load) Error|direct invocation failed' "$output" \
    && { [[ -z $reply_must_contain ]] || grep -Fq "$reply_must_contain" "$output"; }; then
    reply_ok=1
    success=$((success + 1))
  fi
  # Input generation is frozen by the direct-request manifest and sequence
  # index. The current direct invoker does not emit serialized request bytes.
  printf '%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t0\t%s\t%s\t1\n' \
    "$meta_index" "$slot" "$revision" "$scheduled_ns" "$call_start_ns" "$call_end_ns" \
    "$request_ns" "$((call_start_ns - scheduled_ns))" "$status" "$reply_ok" \
    "$reply_bytes" "$reply_sha" >>"$output_dir/invocations.tsv"
done

printf 'FIGURE_9_16_DIRECT_WINDOW profile=%s calls=%s success=%s failed=%s interval_ms=%s elapsed_ms=%s output=%s\n' \
  "$profile" "$calls" "$success" "$((calls - success))" "$interval_ms" \
  "$((( $(date +%s%N) - start_ns ) / 1000000))" "$output_dir"
(( wait_status == 0 && success == calls ))
