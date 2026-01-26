#!/bin/bash

DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null 2>&1 && pwd )"
ROOT="$( cd $DIR && cd ../.. && pwd)"
BINS=$ROOT/bin
CONFIGS=$ROOT/configs/stargz
STARGZ_VERSION=v0.15.1

# Get stargz snapshotter tar
wget --continue --quiet https://github.com/containerd/stargz-snapshotter/releases/download/${STARGZ_VERSION}/stargz-snapshotter-${STARGZ_VERSION}-linux-amd64.tar.gz

# Copy stargz config
sudo mkdir -p /etc/containerd
sudo cp $CONFIGS/config.toml /etc/containerd/config.toml

# Unzip stargz binary and install it
sudo tar -C /usr/local/bin -xvf stargz-snapshotter-${STARGZ_VERSION}-linux-amd64.tar.gz containerd-stargz-grpc ctr-remote

# Download stargz snapshotter service configuration file
sudo wget -O /etc/systemd/system/stargz-snapshotter.service https://raw.githubusercontent.com/containerd/stargz-snapshotter/main/script/config/etc/systemd/system/stargz-snapshotter.service

# Enable stargz snapshotter
sudo systemctl enable --now stargz-snapshotter

# Check if containerd process is running and stop if true
# if sudo screen -list | grep "containerd"; then
#     sudo screen -XS containerd quit
# fi

# Start containerd
# sudo screen -dmS containerd bash -c "containerd > >(tee -a /tmp/vhive-logs/containerd.stdout) 2> >(tee -a /tmp/vhive-logs/containerd.stderr >&2)"