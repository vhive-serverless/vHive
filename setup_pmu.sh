#!/bin/bash

source scripts/setup_system.sh

wget -O go1.17.7.linux-amd64.tar.gz https://go.dev/dl/go1.17.7.linux-amd64.tar.gz
sudo rm -rf /usr/local/go && sudo tar -C /usr/local -xzf go1.17.7.linux-amd64.tar.gz
export PATH=$PATH:/usr/local/go/bin

sudo add-apt-repository ppa:git-core/ppa -y
sudo apt update
sudo apt install git -y

source scripts/setup_firecracker_containerd.sh
go build -race -v -a ./...

#source scripts/install_pmutools.sh || true

#screen -d -m -S firecracker bash -c 'sudo PATH=$PATH /usr/local/bin/firecracker-containerd --config /etc/firecracker-containerd/config.toml'

#echo "---------------------> DONE"
