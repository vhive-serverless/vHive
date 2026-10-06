#!/usr/bin/env bash
# Read-only source scan. Run in its own tmux session on the backend.
set -uo pipefail
run_root=$1
[[ $run_root == /users/Liquidz/streams8/* && -s $run_root/config/plan.json ]] || exit 2
[[ ! -e $run_root/source-inventory.json && ! -e $run_root/scan.rc ]] || exit 3
"$run_root/bin/copy-corpus-streams8" -plan "$run_root/config/plan.json" \
  -inventory "$run_root/source-inventory.json" >"$run_root/scan.log" 2>&1
rc=$?
printf '%s\n' "$rc" >"$run_root/scan.rc"
exit "$rc"
