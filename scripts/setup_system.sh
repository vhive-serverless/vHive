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

# Add skopeo sources
. /etc/os-release
echo "deb https://download.opensuse.org/repositories/devel:/kubic:/libcontainers:/stable/xUbuntu_${VERSION_ID}/ /" | sudo tee /etc/apt/sources.list.d/devel:kubic:libcontainers:stable.list
curl -L https://download.opensuse.org/repositories/devel:/kubic:/libcontainers:/stable/xUbuntu_${VERSION_ID}/Release.key | sudo apt-key add -

sudo apt-get update >> /dev/null

sudo apt-get -y install \
    apt-transport-https \
    ca-certificates \
    curl \
    gcc \
    g++ \
    make \
    acl \
    net-tools \
    git-lfs \
    bc \
    dmsetup \
    gnupg-agent \
    software-properties-common \
    gettext-base \
    skopeo >> /dev/null

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

sudo sysctl -w net.ipv4.conf.all.forwarding=1
# Avoid "neighbour: arp_cache: neighbor table overflow!"
sudo sysctl -w net.ipv4.neigh.default.gc_thresh1=1024
sudo sysctl -w net.ipv4.neigh.default.gc_thresh2=2048
sudo sysctl -w net.ipv4.neigh.default.gc_thresh3=4096
sudo sysctl -w net.ipv4.ip_local_port_range="32769 65535"
sudo sysctl -w kernel.pid_max=4194303
sudo sysctl -w kernel.threads-max=999999999
sudo swapoff -a
sudo sysctl net.ipv4.ip_forward=1
sudo sysctl --system
