#!/bin/bash

# MIT License
#
# Copyright (c) 2020 Dmitrii Ustiugov, Plamen Petrov and EASE lab
#
# Permission is hereby granted, free of charge, to any person obtaining a copy
# of this software and associated documentation files (the "Software"), to deal
# in the Software without restriction, including without limitation the rights
# to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
# copies of the Software, and to permit persons to whom the Software is
# furnished to do so, subject to the following conditions:
#
# The above copyright notice and this permission notice shall be included in all
# copies or substantial portions of the Software.
#
# THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
# IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
# FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
# AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
# LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
# OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
# SOFTWARE.

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
sudo cp $BINS/vmlinux-6.1.141 /var/lib/firecracker-containerd/runtime/hello-vmlinux.bin

sudo cp $CONFIGS/config.toml /etc/firecracker-containerd/

# When executed inside a docker container, this command returns the container ID of the container.
# on a non container environment, this returns "/".
CONTAINERID=$(basename $(cat /proc/1/cpuset))

# Docker container ID is 64 characters long.
if [ 64 -eq ${#CONTAINERID} ]; then
  sudo sed -i "s/fc-dev-thinpool/${CONTAINERID}_thinpool/" /etc/firecracker-containerd/config.toml
fi

sudo cp $CONFIGS/firecracker-runtime.json /etc/containerd/
