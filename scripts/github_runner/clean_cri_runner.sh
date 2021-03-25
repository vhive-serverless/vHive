# MIT License
#
# Copyright (c) 2020 Shyam Jesalpura and EASE lab
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

#!/bin/bash

PWD="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"

STOCK_CONTAINERD=$1

KUBECONFIG=/etc/kubernetes/admin.conf kn service delete --all
if [ "$STOCK_CONTAINERD" == "stock-only" ]; then
    sudo kubeadm reset --cri-socket /run/containerd/containerd.sock -f
else
    sudo kubeadm reset --cri-socket /etc/firecracker-containerd/fccd-cri.sock -f
fi
sudo pkill -INT vhive
sudo pkill -9 firecracker-containerd
sudo pkill -9 firecracker
sudo pkill -9 containerd
$PWD/../create_devmapper.sh
sudo rm /etc/firecracker-containerd/fccd-cri.sock
rm ${HOME}/.kube/config
sudo rm -rf ${HOME}/tmp

echo Cleaning /var/lib/firecracker-containerd/*
for d in containerd shim-base snapshotter; do sudo rm -rf /var/lib/firecracker-containerd/$d; done

echo Cleaning /run/firecracker-containerd/*
sudo rm -rf /run/firecracker-containerd/containerd.sock.ttrpc \
    /run/firecracker-containerd/io.containerd.runtime.v1.linux \
    /run/firecracker-containerd/io.containerd.runtime.v2.task

ifconfig -a | grep _tap | cut -f1 -d":" | while read line ; do sudo ip link delete "$line" ; done
ifconfig -a | grep tap_ | cut -f1 -d":" | while read line ; do sudo ip link delete "$line" ; done
bridge -j vlan |jq -r '.[].ifname'| while read line ; do sudo ip link delete "$line" ; done
