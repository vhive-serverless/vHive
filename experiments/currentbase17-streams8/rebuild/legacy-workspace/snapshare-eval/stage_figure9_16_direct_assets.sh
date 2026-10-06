#!/usr/bin/env bash
set -euo pipefail

script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
workspace_dir=$(cd -- "$script_dir/.." && pwd)
nodes_env=${NODES_ENV:-$workspace_dir/colocation-infra/cluster/nodes.20260830.figure9-16-c6620-3node.env}
# shellcheck source=/dev/null
source "$nodes_env"

worker=${WORKER_NODES[0]}
loader=${LOADER_NODE:?LOADER_NODE is required}
relay_local=${FIGURE_RELAY_LOCAL:-$workspace_dir/.dist/vhive-figure9-16/bin/relay-figure9-16-direct-all}
converter_local=${FIGURE_CONVERTER_LOCAL:-$workspace_dir/.dist/vhive-figure9-16/bin/snapshot-converter-figure9-16}
invoker_local=${FIGURE_DIRECT_INVOKER_LOCAL:-$workspace_dir/.dist/vswarm-direct-invoker/tools/direct-invoker/direct-invoker}
legacy_zip_local=${FIGURE_LEGACY_ZIP_LOCAL:-$workspace_dir/snapshare-5.01-extracted/snapshare/26_05_01_snapshots/snapshots.zip}
legacy_images_local=${FIGURE_LEGACY_IMAGES_LOCAL:-$workspace_dir/snapshare-5.01-extracted/snapshare/26_05_01_snapshots/images}
image_inventory=${FIGURE_IMAGES_MANIFEST:-$script_dir/results/figure9-16/20260827_figure_images_inventory.sha256}
remote_relay=${FIGURE_RELAY_BINARY:-/users/Liquidz/vhive-snapshare/bin/relay-figure9-16-direct-all}
remote_converter=${FIGURE_CONVERTER_BINARY:-/users/Liquidz/vhive-snapshare/bin/snapshot-converter-figure9-16}
remote_helper=${FIGURE_RESTART_HELPER:-/users/Liquidz/figure9-16/restart-worker.sh}
remote_invoker=${FIGURE_DIRECT_INVOKER:-/users/Liquidz/figure9-16/bin/direct-invoker}
legacy_root=${LEGACY_REMOTE_ROOT:-$BACKEND_MOUNT/figure9-16/legacy-source}
remote_image_dir=${FIGURE_REMOTE_IMAGE_ARCHIVE_DIR:-$BACKEND_MOUNT/figure9-16/image-archives}
image_dir=$script_dir/artifacts/function-images/20260818-rebuild-f268d11
image_tag=restore-reconnect-f268d11-r2-20260818-esgz
result_dir=${RESULT_DIR:-$script_dir/results/bootstrap/20260830_figure9_16_assets}

ssh_opts=(-A -oStrictHostKeyChecking=no -oServerAliveInterval=20 -oServerAliveCountMax=6)
rsync_ssh='ssh -A -oStrictHostKeyChecking=no -oServerAliveInterval=20 -oServerAliveCountMax=6'

[[ ${EXPERIMENT_ID:-} == figure9-16-direct-er020-er069-er032 ]] || { echo "unexpected experiment" >&2; exit 2; }
[[ $worker == "$AUTHORIZED_CONTROL_NODE" && $BACKEND_NODE == "$AUTHORIZED_BACKEND_NODE" ]] || exit 2
[[ $loader != "$worker" && $loader != "$BACKEND_NODE" ]] || exit 2
[[ $legacy_root == "$BACKEND_MOUNT"/figure9-16/* ]] || exit 2
[[ $remote_image_dir == "$BACKEND_MOUNT"/figure9-16/* ]] || exit 2
for file in "$relay_local" "$converter_local" "$invoker_local" "$legacy_zip_local" \
  "$legacy_images_local/rootfs.tar" "$image_inventory" \
  "$script_dir/restart_snapshare_xl170_worker.sh"; do
  [[ -s $file ]] || { echo "missing asset: $file" >&2; exit 3; }
done
for workload in image-rotate-go image-rotate-python video-processing-python video-analytics-standalone-python; do
  file=$image_dir/$workload.restore-reconnect-f268d11-r2-20260818.esgz.oci.tar
  [[ -s $file ]] || { echo "missing image archive: $file" >&2; exit 3; }
done

mkdir -p "$result_dir"
cp "$nodes_env" "$result_dir/nodes.env"
{
  sha256sum "$relay_local" "$converter_local" "$invoker_local" "$legacy_zip_local"
  (cd "$legacy_images_local" && {
    sha256sum rootfs.tar
    find . -mindepth 2 -maxdepth 2 -type f -name container.tar -print0 | sort -z | xargs -0 sha256sum
  })
  for workload in image-rotate-go image-rotate-python video-processing-python video-analytics-standalone-python; do
    sha256sum "$image_dir/$workload.restore-reconnect-f268d11-r2-20260818.esgz.oci.tar"
  done
} >"$result_dir/local-assets.sha256"

ssh "${ssh_opts[@]}" "$worker" \
  "mkdir -p /users/Liquidz/vhive-snapshare/bin /users/Liquidz/figure9-16"
scp -q "${ssh_opts[@]}" "$relay_local" "$worker:$remote_relay.new"
scp -q "${ssh_opts[@]}" "$converter_local" "$worker:$remote_converter.new"
scp -q "${ssh_opts[@]}" "$script_dir/restart_snapshare_xl170_worker.sh" "$worker:$remote_helper.new"
ssh "${ssh_opts[@]}" "$worker" \
  "chmod 0755 '$remote_relay.new' '$remote_converter.new' '$remote_helper.new'; mv -f '$remote_relay.new' '$remote_relay'; mv -f '$remote_converter.new' '$remote_converter'; mv -f '$remote_helper.new' '$remote_helper'; sha256sum '$remote_relay' '$remote_converter' '$remote_helper'" \
  >"$result_dir/worker-assets.sha256"

ssh "${ssh_opts[@]}" "$loader" "mkdir -p '$(dirname -- "$remote_invoker")'"
scp -q "${ssh_opts[@]}" "$invoker_local" "$loader:$remote_invoker.new"
ssh "${ssh_opts[@]}" "$loader" \
  "chmod 0755 '$remote_invoker.new'; mv -f '$remote_invoker.new' '$remote_invoker'; sha256sum '$remote_invoker'" \
  >"$result_dir/loader-assets.sha256"

# The portable bootstrap archive has the four newer workloads plus one old
# fibonacci-go alias.  The frozen Figure 9--16 inventory instead needs all
# thirteen legacy images under their canonical archive names.  Preserve that
# non-manifest alias outside the image root, add the legacy images, and require
# an exact rootfs + 17 workload-image inventory before corpus generation.
ssh "${ssh_opts[@]}" "$worker" bash -s -- \
  cd90ec5f0151ea70419e6e75b357f54345b2bdf58fa40ea860f7a817dfb430f2 <<'REMOTE'
set -euo pipefail
extra_sha=$1
source_dir=/users/Liquidz/images/fibonacci-go
quarantine=/users/Liquidz/figure9-16/bootstrap-extra-images/fibonacci-go
if [[ -d $source_dir ]]; then
  test "$(sha256sum "$source_dir/container.tar" | awk '{print $1}')" = "$extra_sha"
  test ! -e "$quarantine"
  mkdir -p "$(dirname -- "$quarantine")"
  mv "$source_dir" "$quarantine"
fi
REMOTE
rsync -a --partial --info=progress2 -e "$rsync_ssh" \
  "$legacy_images_local/" "$worker:/users/Liquidz/images/"
ssh "${ssh_opts[@]}" "$worker" \
  "cd /users/Liquidz/images && { sha256sum rootfs.tar; find . -mindepth 2 -maxdepth 2 -type f -name container.tar -print0 | sort -z | xargs -0 sha256sum; }" \
  >"$result_dir/worker-images.sha256"
cmp -s "$image_inventory" "$result_dir/worker-images.sha256" \
  || { echo "worker image inventory differs from the frozen 17-workload inventory" >&2; exit 4; }

ssh "${ssh_opts[@]}" "$BACKEND_NODE" bash -s -- "$legacy_root" "$remote_image_dir" <<'REMOTE'
set -euo pipefail
sudo install -d -o "$(id -u)" -g "$(id -g)" "$1" "$2"
REMOTE
rsync -a --partial --info=progress2 -e "$rsync_ssh" \
  "$legacy_zip_local" "$BACKEND_NODE:$legacy_root/snapshots.zip"
for workload in image-rotate-go image-rotate-python video-processing-python video-analytics-standalone-python; do
  archive=$image_dir/$workload.restore-reconnect-f268d11-r2-20260818.esgz.oci.tar
  rsync -a --partial -e "$rsync_ssh" "$archive" "$BACKEND_NODE:$remote_image_dir/"
done

ssh "${ssh_opts[@]}" "$BACKEND_NODE" bash -s -- \
  "$legacy_root" "$remote_image_dir" "$SNAPSHARE_REGISTRY_HOST" "$SNAPSHARE_REGISTRY_PORT" "$image_tag" <<'REMOTE' \
  >"$result_dir/backend-assets.log"
set -euo pipefail
legacy_root=$1
archive_dir=$2
registry_host=$3
registry_port=$4
tag=$5
sha256sum "$legacy_root/snapshots.zip"
for workload in image-rotate-go image-rotate-python video-processing-python video-analytics-standalone-python; do
  archive=$archive_dir/$workload.restore-reconnect-f268d11-r2-20260818.esgz.oci.tar
  sha256sum "$archive"
  target="docker://$registry_host:$registry_port/liquidzk/$workload:$tag"
  skopeo copy --dest-tls-verify=false "oci-archive:$archive" "$target" >/dev/null
  skopeo inspect --tls-verify=false "$target" | jq -r '[.Name,.Tag,.Digest] | @tsv'
done
REMOTE

local_legacy_sha=$(sha256sum "$legacy_zip_local" | awk '{print $1}')
remote_legacy_sha=$(ssh "${ssh_opts[@]}" "$BACKEND_NODE" "sha256sum '$legacy_root/snapshots.zip'" | awk '{print $1}')
[[ $local_legacy_sha == "$remote_legacy_sha" ]] || { echo "legacy ZIP checksum mismatch" >&2; exit 4; }
touch "$result_dir/ASSETS_READY"
printf 'FIGURE_9_16_ASSETS_READY worker=%s backend=%s loader=%s\n' "$worker" "$BACKEND_NODE" "$loader"
