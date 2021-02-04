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

apt-get update
apt-get install --yes \
    git \
    sudo \
    apt-utils \
    wget \
    curl \
    make \
    gcc \
    iproute2 \
    iptables \
    apt-transport-https \
    ca-certificates \
    curl \
    gcc \
    g++ \
    make \
    jq \
    acl \
    net-tools \
    git-lfs \
    bc \
    dmsetup \
    gnupg-agent \
    wget \
    tmux \
    vim \
    software-properties-common

# install golang
if [ ! -f "/usr/local/go/bin/go" ]; then
    wget -c https://golang.org/dl/go1.15.linux-amd64.tar.gz
    sudo tar -C /usr/local -xzf go1.15.linux-amd64.tar.gz
    sudo ln -s /usr/local/go/bin/go /usr/bin/go
fi

PWD="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
$PWD/setup_system.sh
$PWD/create_devmapper.sh

sudo apt-get -y install  unzip pkg-config libseccomp-dev unzip tar libseccomp2 socat util-linux apt-transport-https curl ipvsadm >> /dev/null

if [ ! -f "protoc-3.11.4-linux-x86_64.zip" ]; then
    wget -c https://github.com/google/protobuf/releases/download/v3.11.4/protoc-3.11.4-linux-x86_64.zip
    sudo unzip -u protoc-3.11.4-linux-x86_64.zip -d /usr/local
fi

# Install knative CLI
if [ ! -d "$HOME/client" ]; then
    git clone https://github.com/knative/client.git $HOME/client
    cd $HOME/client
    hack/build.sh -f
    sudo cp kn /usr/local/bin
fi

# Necessary for containerd as container runtime but not docker
sudo modprobe overlay
sudo modprobe br_netfilter

# Set up required sysctl params, these persist across reboots.
sudo tee /etc/sysctl.d/99-kubernetes-cri.conf <<EOF
net.bridge.bridge-nf-call-iptables  = 1
net.ipv4.ip_forward                 = 1
net.bridge.bridge-nf-call-ip6tables = 1
EOF

sudo sysctl --system

# setup github runner
cd $HOME
if [ ! -d "$HOME/actions-runner" ]; then
    mkdir actions-runner && cd actions-runner
    curl -O -L -C - https://github.com/actions/runner/releases/download/v2.276.1/actions-runner-linux-x64-2.276.1.tar.gz
    tar xzf ./actions-runner-linux-x64-2.276.1.tar.gz
    chmod +x ./config.sh
    chmod +x ./run.sh
    systemctl enable run-at-startup.service
    sudo tee /etc/systemd/system/connect_github_runner.service <<END
[Unit]
Description=Connect to Github as self hosted runner
Wants=network-online.target
After=network.target network-online.target

[Service]
Type=simple
RemainAfterExit=yes
Environment="RUNNER_ALLOW_RUNASROOT=1"
ExecStart=/root/actions-runner/run.sh
TimeoutStartSec=0

[Install]
WantedBy=default.target
END
else
    systemctl daemon-reload
    sysctl enable connect_github_runner

fi

# we want the command (expected to be systemd) to be PID1, so exec to it
exec "$@"
