#!/bin/bash

DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null 2>&1 && pwd )"

echo Killing firecracker agents and VMs
sudo pkill -9 firec
sudo pkill -9 containerd

echo Resetting iptables
sudo iptables -F
sudo iptables -t nat -F

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

echo Removing devmapper devices
for de in `sudo dmsetup ls| cut -f1|grep snap`; do sudo dmsetup remove $de && echo Removed $de; done
sudo dmsetup remove fc-dev-thinpool

echo Cleaning /var/lib/firecracker-containerd/*
for d in containerd shim-base snapshotter; do sudo rm -rf /var/lib/firecracker-containerd/$d; done

echo Creating a fresh devmapper
source $DIR/create_devmapper.sh

echo Cleaning /run/firecracker-containerd/*
sudo rm -rf /run/firecracker-containerd/containerd.sock.ttrpc \
    /run/firecracker-containerd/io.containerd.runtime.v1.linux \
    /run/firecracker-containerd/io.containerd.runtime.v2.task

echo Cleaning CNI state, e.g., allocated addresses
sudo rm /var/lib/cni/networks/fcnet*/last_reserved_ip.0 || echo clean already
sudo rm /var/lib/cni/networks/fcnet*/19* || echo clean already

