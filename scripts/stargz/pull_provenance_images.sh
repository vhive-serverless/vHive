#!/usr/bin/env bash
# Pull immutable eStargz images on the host and mount their root filesystems as
# content sources for SplitSnap-style provenance classification.
#
# Usage:
#   sudo scripts/stargz/pull_provenance_images.sh IMAGE [IMAGE ...]
#
# Set PROVENANCE_IMAGE_DIR to choose the output directory. Pass that directory
# to vhive_test with -provenanceImageSourcesTest. The script intentionally does
# not remove mounts: the classifier reads them while tests or vHive are running.

set -euo pipefail

if [[ $# -eq 0 ]]; then
  echo "usage: $0 IMAGE [IMAGE ...]" >&2
  exit 2
fi

CTR_BIN="${CTR_BIN:-}"
if [[ -z "$CTR_BIN" ]]; then
  if command -v ctr-remote >/dev/null 2>&1; then
    CTR_BIN="ctr-remote"
  elif command -v ctr >/dev/null 2>&1; then
    CTR_BIN="ctr"
  else
    echo "ctr-remote or ctr is required" >&2
    exit 1
  fi
fi

NAMESPACE="${CONTAINERD_NAMESPACE:-firecracker-containerd}"
SNAPSHOTTER="${STARGZ_SNAPSHOTTER:-stargz}"
PLATFORM="${PLATFORM:-linux/amd64}"
OUTPUT_DIR="${PROVENANCE_IMAGE_DIR:-/var/lib/vhive/provenance-images}"
INDEX="$OUTPUT_DIR/index.tsv"

mkdir -p "$OUTPUT_DIR"
touch "$INDEX"

for image in "$@"; do
  source_id="$(printf '%s' "$image" | sha256sum | awk '{print $1}')"
  mount_dir="$OUTPUT_DIR/$source_id"

  "$CTR_BIN" --namespace "$NAMESPACE" images pull --platform "$PLATFORM" --snapshotter "$SNAPSHOTTER" "$image"
  if ! mountpoint -q "$mount_dir"; then
    mkdir -p "$mount_dir"
    if ! "$CTR_BIN" --namespace "$NAMESPACE" images mount --platform "$PLATFORM" --snapshotter "$SNAPSHOTTER" "$image" "$mount_dir"; then
      # ctr-remote versions built without an unpack-platform matcher reject
      # `images mount` even though the image is already present. The regular
      # ctr client can mount the same stargz snapshot with the explicit
      # platform, so use it as a compatible fallback.
      if [[ "$CTR_BIN" != "ctr" ]] && command -v ctr >/dev/null 2>&1; then
        ctr --namespace "$NAMESPACE" images mount --platform "$PLATFORM" --snapshotter "$SNAPSHOTTER" "$image" "$mount_dir"
      else
        rmdir "$mount_dir" 2>/dev/null || true
        echo "failed to mount $image; set PLATFORM for its manifest platform or use a ctr build with stargz snapshot support" >&2
        exit 1
      fi
    fi
  fi

  # Replace an existing entry atomically enough for the single-writer setup
  # script. Image references cannot contain tabs, so TSV is sufficient here.
  temp_index="$(mktemp "$OUTPUT_DIR/.index.XXXXXX")"
  awk -F '\t' -v image="$image" '$1 != image { print }' "$INDEX" > "$temp_index"
  printf '%s\t%s\n' "$image" "$mount_dir" >> "$temp_index"
  mv "$temp_index" "$INDEX"
  echo "$image -> $mount_dir"
done
