#!/bin/bash

# MIT License
#
# Copyright (c) 2021 Mert Bora Alper and EASE lab
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

# Install base- and setup-dependencies
apt-get update
apt-get install --yes \
    acl \
    apt-transport-https \
    bash-completion \
    bc \
    ca-certificates \
    curl \
    dmsetup \
    g++\
    gcc \
    gettext-base \
    git \
    git-lfs \
    gnupg-agent \
    iproute2 \
    iptables \
    ipvsadm \
    jq \
    libicu67 \
    libseccomp-dev \
    libseccomp2 \
    make \
    nano \
    net-tools \
    pkg-config \
    skopeo \
    socat \
    software-properties-common \
    sudo \
    tar \
    tmux \
    unzip \
    util-linux \
    vim \
    wget

# Install docker-compose
curl -L "https://github.com/docker/compose/releases/download/1.29.2/docker-compose-$(uname -s)-$(uname -m)" -o /usr/local/bin/docker-compose
chmod +x /usr/local/bin/docker-compose

# Install kn
curl -L "https://github.com/knative/client/releases/download/v0.23.2/kn-linux-amd64" -o /usr/local/bin/kn
chmod +x /usr/local/bin/kn
echo 'source /etc/bash_completion' >> ~/.bashrc
echo 'source <(kn completion bash)' >> ~/.bashrc

# Install GitHub Runner
mkdir -p actions-runner
cd actions-runner
LATEST_URL=$(curl -s https://api.github.com/repos/actions/runner/releases/latest | jq -r '.assets[] | select(.name | startswith("actions-runner-linux-x64")) | .browser_download_url')
curl -C - -o actions-runner.tar.gz -L $LATEST_URL
tar xzf actions-runner.tar.gz
./bin/installdependencies.sh
./config.sh --url https://github.com/ease-lab/vhive --labels stock-knative --token $TOKEN

tee /etc/systemd/system/github-runner.service <<END
[Unit]
Description=Connect to Github as self hosted runner
Wants=network-online.target
After=network.target network-online.target

StartLimitIntervalSec=500
StartLimitBurst=5

[Service]
Type=simple
Environment="RUNNER_ALLOW_RUNASROOT=1"
ExecStart=/actions-runner/run.sh
TimeoutStartSec=0
Restart=on-failure
RestartSec=5s

[Install]
WantedBy=default.target
END
systemctl daemon-reload
systemctl enable github-runner --now
