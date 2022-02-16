#!/bin/bash

scripts/setup_system.sh

wget https://go.dev/dl/go1.17.7.linux-amd64.tar.gz
sudo rm -rf /usr/local/go && sudo tar -C /usr/local -xzf go1.17.7.linux-amd64.tar.gz
export PATH=$PATH:/usr/local/go/bin

sudo add-apt-repository ppa:git-core/ppa -y
sudo apt update
sudo apt install git -y

scripts/setup_firecracker_containerd.sh
go build -race -v -a ./...

scripts/install_pmutools.sh

screen -d -m sudo PATH=$PATH /usr/local/bin/firecracker-containerd --config /etc/firecracker-containerd/config.toml
