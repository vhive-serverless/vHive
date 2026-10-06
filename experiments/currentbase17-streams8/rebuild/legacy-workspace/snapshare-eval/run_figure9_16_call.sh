#!/usr/bin/env bash
set -euo pipefail

if (( $# != 2 )); then
  echo "usage: $0 <profile> <source|source-local|profile|target-remote>" >&2
  exit 2
fi

profile=$1
stage=$2
case $stage in source|source-local|profile|target-remote) ;; *) exit 2 ;; esac

script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
workspace_dir=$(cd -- "$script_dir/.." && pwd)
workloads_json=${FIGURE_WORKLOADS_JSON:-$script_dir/configs/figure9_16/workloads.json}
direct_requests_json=${FIGURE_DIRECT_REQUESTS_JSON:-$script_dir/configs/figure9_16/direct_requests.json}
nodes_env=${NODES_ENV:-$workspace_dir/colocation-infra/cluster/nodes.20260830.figure9-16-c6620-3node.env}
minio_container_override=${SNAPSHARE_MINIO_CONTAINER:-}
minio_endpoint=${SNAPSHARE_MINIO_ENDPOINT:-}
minio_user=${SNAPSHARE_MINIO_USER:-minio}
minio_password=${SNAPSHARE_MINIO_PASSWORD:-minio123}
# shellcheck source=/dev/null
source "$nodes_env"

worker=${WORKER_NODES[0]}
worker_ip=${WORKER_PRIVATE_IPS[0]}
loader=${LOADER_NODE:-$BACKEND_NODE}
mc_node=${SNAPSHARE_MC_NODE:-$BACKEND_NODE}
minio_container=$minio_container_override
if [[ -z $minio_endpoint && -z $minio_container ]]; then
  echo "set SNAPSHARE_MINIO_ENDPOINT or SNAPSHARE_MINIO_CONTAINER" >&2
  exit 2
fi
ready_object=${SNAPSHARE_READY_OBJECT:-snap_file}
upload_timeout=${SNAPSHOT_UPLOAD_TIMEOUT_SEC:-3600}
result_csv=${RESULT_CSV:-}
run_index=${RUN_INDEX:-0}
direct_invoker=${FIGURE_DIRECT_INVOKER:-/users/Liquidz/figure9-16/bin/direct-invoker}

entry=$(jq -ce --arg profile "$profile" '.workloads[] | select(.profile == $profile)' "$workloads_json")
direct_entry=$(jq -ce --arg profile "$profile" '.requests[] | select(.profile == $profile)' "$direct_requests_json")
snapshot=$(jq -r '.snapshot' <<<"$entry")
image=$(jq -r '.image' <<<"$entry")
function_args=$(jq -r '.function_args // ""' <<<"$entry")
function_env=$(jq -r '.function_env // ""' <<<"$entry")
function_port=$(jq -r '.function_port' <<<"$entry")
function_name=$(jq -r '.function_name' <<<"$direct_entry")
generator=$(jq -r '.generator' <<<"$direct_entry")
value=$(jq -r '.value // ""' <<<"$direct_entry")
function_method=$(jq -r '.function_method // "default"' <<<"$direct_entry")
reply_must_contain=$(jq -r '.reply_must_contain // ""' <<<"$direct_entry")
lower_bound=$(jq -r '.lower_bound // 1' <<<"$direct_entry")
upper_bound=$(jq -r '.upper_bound // 10' <<<"$direct_entry")
seed=$(jq -r '.seed // empty' <<<"$direct_entry")
[[ -n $seed ]] || seed=$(jq -r '.seed' "$direct_requests_json")
function_args=${function_args//__BACKEND_PRIVATE_IP__/$BACKEND_PRIVATE_IP}

[[ $snapshot =~ ^[a-zA-Z0-9._-]+$ ]] || exit 2
[[ $function_port =~ ^[1-9][0-9]*$ ]] || exit 2
[[ $function_name =~ ^[a-zA-Z0-9._-]+$ ]] || exit 2
[[ $generator =~ ^(fixed|unique|linear|random)$ ]] || exit 2
[[ $lower_bound =~ ^[0-9]+$ && $upper_bound =~ ^[0-9]+$ ]] || exit 2
[[ $seed =~ ^-?[0-9]+$ && $run_index =~ ^[0-9]+$ ]] || exit 2
if [[ -n $minio_endpoint ]]; then
  [[ $minio_endpoint =~ ^https?://[a-zA-Z0-9._:-]+$ ]] || exit 2
  [[ $minio_user =~ ^[a-zA-Z0-9._-]+$ && $minio_password =~ ^[a-zA-Z0-9._-]+$ ]] || exit 2
else
  [[ $minio_container =~ ^[a-zA-Z0-9][a-zA-Z0-9_.-]*$ ]] || exit 2
fi
[[ $ready_object =~ ^[a-zA-Z0-9][a-zA-Z0-9_.-]*$ ]] || exit 2
[[ $upload_timeout =~ ^[1-9][0-9]*$ ]] || exit 2
[[ $function_args != *'__BACKEND_PRIVATE_IP__'* ]] || exit 2

ssh_opts=(-A -oStrictHostKeyChecking=no -oServerAliveInterval=20 -oServerAliveCountMax=6)
snapshot_path=/users/Liquidz/snapshots/$snapshot

minio_has() {
	local object=${1:-$ready_object}
	if [[ -n $minio_endpoint ]]; then
		ssh "${ssh_opts[@]}" "$mc_node" \
			"mc alias set eval '$minio_endpoint' '$minio_user' '$minio_password' >/dev/null; mc stat 'eval/snapshots/$snapshot/$object' >/dev/null 2>&1" </dev/null
	else
		ssh "${ssh_opts[@]}" "$BACKEND_NODE" \
			"sudo docker exec '$minio_container' sh -lc 'mc alias set eval http://127.0.0.1:9000 minio minio123 >/dev/null; mc stat eval/snapshots/$snapshot/$object >/dev/null 2>&1'" </dev/null
	fi
}

case $stage in
  source)
    ! ssh "${ssh_opts[@]}" "$worker" "test -e '$snapshot_path'" </dev/null \
      || { echo "local source snapshot already exists" >&2; exit 3; }
    ! minio_has || { echo "remote source snapshot already exists" >&2; exit 3; }
    ;;
	source-local)
    ssh "${ssh_opts[@]}" "$worker" "test -e '$snapshot_path'" </dev/null \
      || { echo "source-local cache is absent" >&2; exit 3; }
		minio_has || { echo "remote source snapshot is incomplete" >&2; exit 3; }
		;;
	profile)
		(( run_index >= 0 && run_index <= 4 )) || { echo "paper-aligned profile index must be 0..4" >&2; exit 3; }
		! minio_has mem_file || { echo "profile revision was committed before request 5" >&2; exit 3; }
		! minio_has working_set_pages || { echo "profile working set was published before request 5" >&2; exit 3; }
		if (( run_index == 0 )); then
			! ssh "${ssh_opts[@]}" "$worker" "test -e '$snapshot_path'" </dev/null \
				|| { echo "new profiling snapshot already exists locally" >&2; exit 3; }
		else
			ssh "${ssh_opts[@]}" "$worker" "test -d '$snapshot_path'" </dev/null \
				|| { echo "active profiling session is absent" >&2; exit 3; }
		fi
		;;
  target-remote)
    ! ssh "${ssh_opts[@]}" "$worker" "test -e '$snapshot_path'" </dev/null \
      || { echo "target-remote cache is not cold" >&2; exit 3; }
    minio_has || { echo "remote snapshot is incomplete" >&2; exit 3; }
    ;;
esac

encode_arg() {
  local encoded
  encoded=$(printf '%s' "$1" | base64 -w0)
  printf '%s' "${encoded:-__SNAPSHARE_EMPTY__}"
}
function_args_b64=$(encode_arg "$function_args")
function_env_b64=$(encode_arg "$function_env")
value_b64=$(encode_arg "$value")
method_b64=$(encode_arg "$function_method")

set +e
call_output=$(ssh "${ssh_opts[@]}" "$loader" bash -s -- \
  "$direct_invoker" "$worker_ip" "$image" "$snapshot-a-b" "$function_args_b64" \
  "$function_env_b64" "$function_port" "$function_name" "$generator" "$value_b64" \
  "$method_b64" "$lower_bound" "$upper_bound" "$seed" "$run_index" <<'REMOTE' 2>&1
set -euo pipefail
direct_invoker=$1
worker_ip=$2
image=$3
revision=$4
decode_arg() {
  if [[ $1 == __SNAPSHARE_EMPTY__ ]]; then
    printf ''
  else
    printf '%s' "$1" | base64 -d
  fi
}
function_args=$(decode_arg "$5")
function_env=$(decode_arg "$6")
function_port=$7
function_name=$8
generator=$9
shift 9
value=$(decode_arg "$1")
function_method=$(decode_arg "$2")
lower_bound=$3
upper_bound=$4
seed=$5
run_index=$6
test -x "$direct_invoker"

set +e
response=$("$direct_invoker" \
  --address "$worker_ip:8080" \
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
  --sequence-index "$run_index" \
  --timeout 900s 2>&1)
status=$?
set -e
printf '%s\n' "$response"
request_ns=$(sed -n 's/^DIRECT_REQUEST_NS=\([0-9][0-9]*\).*/\1/p' <<<"$response" | tail -n 1)
[[ -n $request_ns ]] || request_ns=0
printf 'E2E_PRECISE_NS=%s EXIT=%s DIRECT_NATIVE=1\n' "$request_ns" "$status"
exit "$status"
REMOTE
)
call_status=$?
set -e
printf '%s\n' "$call_output"
(( call_status == 0 )) || exit "$call_status"
! grep -Eq 'Error:|Failed|Server Error|Snapshot (Download|Load) Error' <<<"$call_output" || exit 4
[[ -z $reply_must_contain ]] || grep -Fq "$reply_must_contain" <<<"$call_output" || exit 4
grep -Eq 'DIRECT_REQUEST_NS=[1-9][0-9]*' <<<"$call_output" || exit 4
grep -Eq 'E2E_PRECISE_NS=[1-9][0-9]* EXIT=0 DIRECT_NATIVE=1' <<<"$call_output" || exit 4

if [[ $stage == source ]]; then
  ready=false
  for _ in $(seq 1 "$upload_timeout"); do
    if minio_has; then ready=true; break; fi
    sleep 1
  done
  [[ $ready == true ]] || { echo "snapshot upload timeout" >&2; exit 5; }
fi

if [[ $stage == profile && $run_index == 4 ]]; then
	ready=false
	for _ in $(seq 1 "$upload_timeout"); do
		if minio_has mem_file && minio_has snap_file && minio_has working_set_pages; then
			ready=true
			break
		fi
		sleep 1
	done
	[[ $ready == true ]] || { echo "paper-aligned snapshot/working-set upload timeout" >&2; exit 5; }
fi

if [[ -n $result_csv ]]; then
  e2e_ns=$(sed -n 's/^E2E_PRECISE_NS=\([0-9][0-9]*\).*/\1/p' <<<"$call_output" | tail -n 1)
  printf '%s,%s,%s,%s,%s,%s\n' "$(date -Is)" "$profile" "$stage" "$run_index" "$e2e_ns" "$call_status" >>"$result_csv"
fi
