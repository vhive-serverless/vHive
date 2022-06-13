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

DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null 2>&1 && pwd )"

echo Killing firecracker agents and VMs
sudo pkill -9 firec
sudo pkill -9 containerd

echo Resetting nftables
sudo nft flush table ip filter
sudo nft "add chain ip filter FORWARD { type filter hook forward priority 0; policy accept; }"
sudo nft "add rule ip filter FORWARD ct state related,established counter accept"

echo Deleting veth* devices created by CNI
cat /proc/net/dev | grep veth | cut -d" " -f1| cut -d":" -f1 | while read in; do sudo ip link delete "$in"; done

ifconfig -a | grep _tap | cut -f1 -d":" | while read line ; do sudo ip link delete "$line" ; done
ifconfig -a | grep tap_ | cut -f1 -d":" | while read line ; do sudo ip link delete "$line" ; done
sudo ip link delete br0
sudo ip link delete br1

for i in `seq 0 100`; do sudo ip link delete ${i}_0_tap  1>/dev/null 2>&1; done

echo Cleaning in /var/lib/cni/ non-network
for d in `find /var/lib/cni/ -mindepth 1 -maxdepth 1  -type d | grep -v networks`; do
    sudo rm -rf $d
done


# When executed inside a docker container, this command returns the container ID of the container.
# on a non container environment, this returns "/".
CONTAINERID=$(basename $(cat /proc/1/cpuset))

# Docker container ID is 64 characters long.
if [ 64 -eq ${#CONTAINERID} ]; then
    echo Removing devmapper devices for the current container
    for de in `sudo dmsetup ls| cut -f1|grep $CONTAINERID |grep snap`; do sudo dmsetup remove $de && echo Removed $de; done
    sudo dmsetup remove "${CONTAINERID}_thinpool"
else
    echo Removing devmapper devices
    for de in `sudo dmsetup ls| cut -f1|grep thinpool`; do sudo dmsetup remove $de && echo Removed $de; done
    sudo dmsetup remove fc-dev-thinpool
fi


echo Cleaning /var/lib/firecracker-containerd/*
for d in containerd shim-base snapshotter; do sudo rm -rf /var/lib/firecracker-containerd/$d; done

echo Cleaning /run/firecracker-containerd/*
sudo rm -rf /run/firecracker-containerd/containerd.sock.ttrpc \
    /run/firecracker-containerd/io.containerd.runtime.v1.linux \
    /run/firecracker-containerd/io.containerd.runtime.v2.task \
    /run/containerd/*

echo Cleaning CNI state, e.g., allocated addresses
sudo rm /var/lib/cni/networks/fcnet*/last_reserved_ip.0 || echo clean already
sudo rm /var/lib/cni/networks/fcnet*/19* || echo clean already

echo Cleaning snapshots
sudo rm -rf /fccd/snapshots/*

echo Creating a fresh devmapper
source $DIR/create_devmapper.sh
