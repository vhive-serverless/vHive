#!/bin/bash

sudo ip link set br0 down
sudo ip link set br1 down
sudo brctl delbr br0
sudo brctl delbr br1
sudo rm /etc/firecracker-containerd/fccd-cri.sock
