#!/usr/bin/env bash
# Build source targets only. Does not install services or access experiment nodes.
set -euo pipefail

if [[ $# != 2 ]]; then
  echo "Usage: bash build.sh NEW_ABSOLUTE_OUTPUT_DIR PINNED_DIRECT_INVOKER_REPO" >&2
  exit 2
fi
script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
repo=$(cd -- "$script_dir/../../.." && pwd)
output=$1
invoker_repo=$(cd -- "$2" && pwd)
[[ $output == /* && ! -e $output ]] || { echo 'Output must be a new absolute directory' >&2; exit 2; }
[[ $(git -C "$invoker_repo" rev-parse HEAD) == a096cc43b84e17d57a9cea1f0a35f17a479a2db6 ]] || {
  echo 'Use the pinned snapshare-direct-invoker revision' >&2; exit 2;
}
mkdir -p "$output"
export GOTOOLCHAIN=go1.26.7 CGO_ENABLED=1 GOOS=linux GOARCH=amd64 GOAMD64=v1
cd "$repo"
build() {
  printf 'BUILD %s\n' "$1"
  go build -mod=readonly -buildvcs=false -o "$output/$1" "$2"
}
build relay-streams8 ./cmd/relay
build snapshot-converter-streams8 ./cmd/snapshot_converter
build materialize-full-dedup-streams8 ./cmd/materialize_full_dedup_oracle_ws
build copy-corpus-streams8 ./experiments/currentbase17-streams8/copycorpus
build transcode-ws-streams8 ./experiments/currentbase17-streams8/transcode
build copy-view-streams8 ./experiments/currentbase17-streams8/viewcopy
for target in inventory capacity footprint payload aliases localcache negativews stopowned; do
  build "$target-streams8" "./experiments/currentbase17-streams8/$target"
done
cd "$invoker_repo/tools/direct-invoker"
build direct-invoker .
printf 'BUILD_COMPLETE %s\n' "$output"
