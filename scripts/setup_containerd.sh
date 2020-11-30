#!/bin/bash

set -e

DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null 2>&1 && pwd )"
ROOT="$( cd $DIR && cd .. && pwd)"
BINS=$ROOT/bin
CONFIGS=$ROOT/configs/firecracker-containerd

sudo mkdir -p /etc/firecracker-containerd
sudo mkdir -p /var/lib/firecracker-containerd/runtime
sudo mkdir -p /etc/containerd/

cd $ROOT
git lfs pull

DST=/usr/local/bin

for BINARY in firecracker jailer containerd-shim-aws-firecracker firecracker-containerd firecracker-ctr
do
  sudo cp $BINS/$BINARY $DST
done

# rootfs image
sudo cp $BINS/default-rootfs.img /var/lib/firecracker-containerd/runtime/
# kernel image
sudo curl -fsSL -o /var/lib/firecracker-containerd/runtime/hello-vmlinux.bin https://s3.amazonaws.com/spec.ccfc.min/img/hello/kernel/hello-vmlinux.bin

sudo cp $CONFIGS/config.toml /etc/firecracker-containerd/
sudo cp $CONFIGS/firecracker-runtime.json /etc/containerd/
