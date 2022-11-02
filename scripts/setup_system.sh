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

 . /etc/os-release
sudo apt-get -y install curl ca-certificates >> /dev/null
sudo add-apt-repository -y universe >> /dev/null
sudo apt-get update >> /dev/null

sudo apt-get -y install \
    apt-transport-https \
    ca-certificates \
    curl \
    e2fsprogs \
    util-linux \
    gcc \
    g++ \
    make \
    acl \
    net-tools \
    bc \
    gettext-base \
    jq \
    dmsetup \
    gnupg-agent \
    software-properties-common \
    iproute2 \
    thin-provisioning-tools \
    nftables \
    git-lfs \
    git-lfs >> /dev/null



# stack size, # of open files, # of pids
sudo sh -c "echo \"* soft nofile 1000000\" >> /etc/security/limits.conf"
sudo sh -c "echo \"* hard nofile 1000000\" >> /etc/security/limits.conf"
sudo sh -c "echo \"root soft nofile 1000000\" >> /etc/security/limits.conf"
sudo sh -c "echo \"root hard nofile 1000000\" >> /etc/security/limits.conf"
sudo sh -c "echo \"* soft nproc 4000000\" >> /etc/security/limits.conf"
sudo sh -c "echo \"* hard nproc 4000000\" >> /etc/security/limits.conf"
sudo sh -c "echo \"root soft nproc 4000000\" >> /etc/security/limits.conf"
sudo sh -c "echo \"root hard nproc 4000000\" >> /etc/security/limits.conf"
sudo sh -c "echo \"* soft stack 65536\" >> /etc/security/limits.conf"
sudo sh -c "echo \"* hard stack 65536\" >> /etc/security/limits.conf"
sudo sh -c "echo \"root soft stack 65536\" >> /etc/security/limits.conf"
sudo sh -c "echo \"root hard stack 65536\" >> /etc/security/limits.conf"

sudo sysctl --quiet -w net.ipv4.conf.all.forwarding=1
# Avoid "neighbour: arp_cache: neighbor table overflow!"
sudo sysctl --quiet -w net.ipv4.neigh.default.gc_thresh1=1024
sudo sysctl --quiet -w net.ipv4.neigh.default.gc_thresh2=2048
sudo sysctl --quiet -w net.ipv4.neigh.default.gc_thresh3=4096
sudo sysctl --quiet -w net.ipv4.ip_local_port_range="32769 65535"
sudo sysctl --quiet -w kernel.pid_max=4194303
sudo sysctl --quiet -w kernel.threads-max=999999999
sudo swapoff -a >> /dev/null
sudo sysctl --quiet net.ipv4.ip_forward=1
sudo sysctl --quiet --system

# NAT setup
hostiface=$(sudo route | grep default | tr -s ' ' | cut -d ' ' -f 8)
sudo nft "add table ip filter"
sudo nft "add chain ip filter FORWARD { type filter hook forward priority 0; policy accept; }"
sudo nft "add rule ip filter FORWARD ct state related,established counter accept"
sudo nft "add table ip nat"
sudo nft "add chain ip nat POSTROUTING { type nat hook postrouting priority 0; policy accept; }"
sudo nft "add rule ip nat POSTROUTING oifname ${hostiface} counter masquerade"