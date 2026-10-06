#!/usr/bin/env bash
# Derive a fresh guest from the already-published SquashFS base; no node writes.
# Run the whole script under sudo or fakeroot to preserve owners/device nodes.
set -euo pipefail
if [[ $# != 3 ]]; then
  echo 'Usage: fakeroot bash build-guest.sh BASE.img NEW_OUTPUT.img USER_PUBLIC_KEY.pub' >&2
  exit 2
fi
base=$(realpath -- "$1")
output=$(realpath -m -- "$2")
public_key=$(realpath -- "$3")
[[ ! -e $output && -s $base && -s $public_key ]] || exit 2
[[ $(id -u) == 0 ]] || { echo 'Use sudo or fakeroot for the entire script' >&2; exit 2; }
ssh-keygen -lf "$public_key" >/dev/null
if rg -q 'PRIVATE KEY' "$public_key"; then
  echo 'Pass a public key, never a private key' >&2; exit 2
fi
guest_stage=$(mktemp -d /tmp/splitsnap-guest.XXXXXX)
unsquashfs -d "$guest_stage/rootfs" "$base"
config=$guest_stage/rootfs/etc/containerd-stargz-grpc/config.toml
if rg -q 'docker-registry.registry.svc.cluster.local:5000' "$config"; then
  echo "Base already contains registry configuration; inspect $config" >&2; exit 2
fi
# Append only to the newly extracted generated filesystem.
printf '\n[[resolver.host."docker-registry.registry.svc.cluster.local:5000".mirrors]]\nhost = "docker-registry.registry.svc.cluster.local:5000"\ninsecure = true\n' >> "$config"
install -d -m 0700 "$guest_stage/rootfs/root/.ssh"
install -m 0600 "$public_key" "$guest_stage/rootfs/root/.ssh/authorized_keys"
test -c "$guest_stage/rootfs/dev/console"
test -c "$guest_stage/rootfs/dev/null"
mksquashfs "$guest_stage/rootfs" "$output" -noappend
printf 'GUEST_BUILT %s\nEXTRACTED_TREE %s/rootfs\n' "$output" "$guest_stage"
echo 'Use this same filesystem for rootfs.tar classification; regenerate snapshots with this guest.'
