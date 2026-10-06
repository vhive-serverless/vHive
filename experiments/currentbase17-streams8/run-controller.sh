#!/usr/bin/env bash
# Local tmux entry; a controller attempt never reuses its own log/rc directory.
set -euo pipefail
(( $# >= 3 )) || { echo "usage: $0 ATTEMPT_DIR --output RESULTS [--only system:profile:mode]" >&2; exit 2; }
controller_dir=$1
shift
script_dir=$(cd "$(dirname "$0")" && pwd)
[[ $controller_dir == "$script_dir"/results/*/controller-* ]] || exit 2
mkdir "$controller_dir"
printf '%q ' python3 -u "$script_dir/run_matrix.py" "$@" > "$controller_dir/argv.txt"
set +e
python3 -u "$script_dir/run_matrix.py" "$@" > "$controller_dir/stdout.log" 2> "$controller_dir/stderr.log"
result=$?
printf '%s\n' "$result" > "$controller_dir/rc.tmp"
mv "$controller_dir/rc.tmp" "$controller_dir/rc"
exit "$result"
