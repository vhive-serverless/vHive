#!/usr/bin/env bash
# Rebuild and archive the four restore-safe SnapShare function images.
#
# This script is intended to run on one canonical x86-64 build node. It builds
# each normal image once, converts it to eStargz, pushes both variants to a
# loopback-only temporary registry, and exports both as OCI archives. The
# archives can then be copied off the ephemeral node and imported elsewhere.

set -euo pipefail

source_commit=${SOURCE_COMMIT:-f268d114706f052124f83efd43a5e70f4d810b5b}
image_tag=${IMAGE_TAG:-restore-reconnect-f268d11-r2-20260818}
repo_url=${VSWARM_REPO_URL:-https://github.com/Liquidzk/vSwarm.git}
work_root=${WORK_ROOT:-"$HOME/snapshare-image-rebuild"}
registry=${BUILD_REGISTRY:-127.0.0.1:5000}
registry_container=${REGISTRY_CONTAINER:-snapshare-build-registry}
stargz_version=${STARGZ_VERSION:-0.12.1}

src_dir="$work_root/vSwarm-$source_commit"
bin_dir="$work_root/bin"
archive_dir="$work_root/artifacts/$image_tag"
log_dir="$archive_dir/logs"
ctr_remote="$bin_dir/ctr-remote"

mkdir -p "$bin_dir" "$archive_dir" "$log_dir"

exec > >(tee -a "$log_dir/rebuild.log") 2>&1

echo "REBUILD_START=$(date --iso-8601=seconds)"
echo "SOURCE_COMMIT=$source_commit"
echo "IMAGE_TAG=$image_tag"
echo "WORK_ROOT=$work_root"
echo "REGISTRY=$registry"

sudo apt-get update
sudo DEBIAN_FRONTEND=noninteractive apt-get install -y \
  ca-certificates curl docker-buildx git git-lfs jq skopeo

if [[ ! -x $ctr_remote ]]; then
  archive="$work_root/stargz-snapshotter-v${stargz_version}-linux-amd64.tar.gz"
  curl -fL --retry 5 --retry-all-errors \
    -o "$archive" \
    "https://github.com/containerd/stargz-snapshotter/releases/download/v${stargz_version}/stargz-snapshotter-v${stargz_version}-linux-amd64.tar.gz"
  tar -xzf "$archive" -C "$bin_dir" ctr-remote
  chmod +x "$ctr_remote"
fi

if [[ ! -d $src_dir/.git ]]; then
  git clone "$repo_url" "$src_dir"
fi
git -C "$src_dir" fetch origin "$source_commit"
git -C "$src_dir" checkout --detach "$source_commit"
git -C "$src_dir" lfs install --local
git -C "$src_dir" lfs pull
test "$(git -C "$src_dir" rev-parse HEAD)" = "$source_commit"

if sudo docker container inspect "$registry_container" >/dev/null 2>&1; then
  sudo docker start "$registry_container" >/dev/null
else
  sudo docker run -d \
    --name "$registry_container" \
    --restart unless-stopped \
    -p 127.0.0.1:5000:5000 \
    registry:2.8.3 >/dev/null
fi

for _ in $(seq 1 30); do
  if curl -fsS "http://$registry/v2/" >/dev/null; then
    break
  fi
  sleep 1
done
curl -fsS "http://$registry/v2/" >/dev/null

sudo docker pull docker.io/vhiveease/python-slim:latest
sudo docker pull docker.io/vhiveease/golang-builder:latest

{
  echo "build_timestamp=$(date --iso-8601=seconds)"
  echo "source_repo=$repo_url"
  echo "source_commit=$source_commit"
  echo "image_tag=$image_tag"
  echo "architecture=$(uname -m)"
  echo "kernel=$(uname -srmo)"
  echo "docker_server=$(sudo docker version --format '{{.Server.Version}}')"
  echo "containerd_server=$(sudo ctr version 2>/dev/null | awk '/Server:/{found=1; next} found && /Version:/{print $2; exit}')"
  echo "ctr_remote=$($ctr_remote version 2>&1 | tr '\n' ' ')"
  echo "python_slim=$(sudo docker image inspect docker.io/vhiveease/python-slim:latest --format '{{index .RepoDigests 0}}')"
  echo "golang_builder=$(sudo docker image inspect docker.io/vhiveease/golang-builder:latest --format '{{index .RepoDigests 0}}')"
  echo "vswarm_proto_main=$(git ls-remote https://github.com/vhive-serverless/vSwarm-proto.git refs/heads/main | awk '{print $1}')"
} > "$archive_dir/provenance.txt"

workloads=(
  image-rotate-go
  image-rotate-python
  video-processing-python
  video-analytics-standalone-python
)

declare -A targets=(
  [image-rotate-go]=imageRotateGo
  [image-rotate-python]=imageRotatePython
  [video-processing-python]=videoProcessingPython
  [video-analytics-standalone-python]=videoAnalyticsStandalonePython
)

declare -A dockerfiles=(
  [image-rotate-go]=benchmarks/image-rotate/docker/Dockerfile
  [image-rotate-python]=benchmarks/image-rotate/docker/Dockerfile
  [video-processing-python]=benchmarks/video-processing/docker/Dockerfile
  [video-analytics-standalone-python]=benchmarks/video-analytics-standalone/docker/Dockerfile
)

printf '%s\n' \
  'workload,variant,reference,manifest_digest,archive,archive_bytes,archive_sha256,stargz_layers,total_layers' \
  > "$archive_dir/manifest.csv"

for workload in "${workloads[@]}"; do
  normal_ref="$registry/liquidzk/$workload:$image_tag"
  esgz_ref="$registry/liquidzk/$workload:$image_tag-esgz"
  normal_archive="$archive_dir/$workload.$image_tag.normal.oci.tar"
  esgz_archive="$archive_dir/$workload.$image_tag.esgz.oci.tar"

  echo "BUILD_START workload=$workload target=${targets[$workload]}"
  sudo env DOCKER_BUILDKIT=1 docker build \
    --progress=plain \
    --provenance=false \
    --target "${targets[$workload]}" \
    --tag "$normal_ref" \
    --file "$src_dir/${dockerfiles[$workload]}" \
    "$src_dir" \
    2>&1 | tee "$log_dir/$workload.docker-build.log"
  sudo docker push "$normal_ref" \
    2>&1 | tee "$log_dir/$workload.docker-push.log"

  # A retry may reuse this exact temporary tag. Remove only its generated
  # containerd references; registry blobs and Docker build cache stay intact.
  sudo "$ctr_remote" -n k8s.io images remove "$normal_ref" "$esgz_ref" \
    >/dev/null 2>&1 || true
  sudo "$ctr_remote" -n k8s.io image pull --plain-http "$normal_ref"
  sudo "$ctr_remote" -n k8s.io image optimize --oci "$normal_ref" "$esgz_ref"
  sudo "$ctr_remote" -n k8s.io image push --plain-http "$esgz_ref"

  rm -f "$normal_archive" "$esgz_archive"
  # Preserve the registry manifest bytes as well as its layers. Without this,
  # skopeo converts a Docker v2 normal manifest to OCI during export and the
  # archive can no longer reproduce the recorded registry digest.
  skopeo copy --preserve-digests --src-tls-verify=false \
    "docker://$normal_ref" "oci-archive:$normal_archive:$image_tag"
  skopeo copy --preserve-digests --src-tls-verify=false \
    "docker://$esgz_ref" "oci-archive:$esgz_archive:$image_tag-esgz"

  normal_digest=$(skopeo inspect --tls-verify=false --format '{{.Digest}}' "docker://$normal_ref")
  esgz_digest=$(skopeo inspect --tls-verify=false --format '{{.Digest}}' "docker://$esgz_ref")
  normal_report=$(skopeo inspect --tls-verify=false --raw "docker://$normal_ref" | jq -r '(.layers | length) as $total | ([.layers[] | select(.annotations["containerd.io/snapshot/stargz/toc.digest"] != null)] | length) as $a | [$a, $total] | @tsv')
  esgz_report=$(skopeo inspect --tls-verify=false --raw "docker://$esgz_ref" | jq -r '(.layers | length) as $total | ([.layers[] | select(.annotations["containerd.io/snapshot/stargz/toc.digest"] != null)] | length) as $a | [$a, $total] | @tsv')

  read -r normal_stargz normal_layers <<< "$normal_report"
  read -r esgz_stargz esgz_layers <<< "$esgz_report"
  if [[ $normal_stargz != 0 ]]; then
    echo "normal image unexpectedly has eStargz annotations: $normal_ref" >&2
    exit 1
  fi
  if [[ $esgz_stargz == 0 || $esgz_stargz != "$esgz_layers" ]]; then
    echo "not every eStargz layer has a TOC annotation: $esgz_ref ($esgz_stargz/$esgz_layers)" >&2
    exit 1
  fi

  for variant in normal esgz; do
    if [[ $variant == normal ]]; then
      ref=$normal_ref
      digest=$normal_digest
      archive=$normal_archive
      stargz_layers=$normal_stargz
      total_layers=$normal_layers
    else
      ref=$esgz_ref
      digest=$esgz_digest
      archive=$esgz_archive
      stargz_layers=$esgz_stargz
      total_layers=$esgz_layers
    fi
    archive_bytes=$(stat -c '%s' "$archive")
    archive_sha256=$(sha256sum "$archive" | awk '{print $1}')
    printf '%s,%s,%s,%s,%s,%s,%s,%s,%s\n' \
      "$workload" "$variant" "$ref" "$digest" "$(basename "$archive")" \
      "$archive_bytes" "$archive_sha256" "$stargz_layers" "$total_layers" \
      >> "$archive_dir/manifest.csv"
  done

  if [[ $workload != image-rotate-go ]]; then
    sudo docker run --rm --entrypoint python3 "$normal_ref" -m pip freeze \
      > "$archive_dir/$workload.pip-freeze.txt"
  fi

  echo "BUILD_DONE workload=$workload normal=$normal_digest esgz=$esgz_digest"
done

sha256sum "$archive_dir"/*.oci.tar > "$archive_dir/SHA256SUMS"
cp "$src_dir/benchmarks/image-rotate/docker/Dockerfile" \
  "$archive_dir/Dockerfile.image-rotate"
cp "$src_dir/benchmarks/video-processing/docker/Dockerfile" \
  "$archive_dir/Dockerfile.video-processing"
cp "$src_dir/benchmarks/video-analytics-standalone/docker/Dockerfile" \
  "$archive_dir/Dockerfile.video-analytics-standalone"

echo "REBUILD_COMPLETE=$(date --iso-8601=seconds)"
echo "ARTIFACT_DIR=$archive_dir"
