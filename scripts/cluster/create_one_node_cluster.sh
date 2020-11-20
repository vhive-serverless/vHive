#!/bin/bash
DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null 2>&1 && pwd )"
ROOT="$( cd $DIR && cd .. && cd .. && pwd)"

# Create kubelet service
$DIR/setup_worker_kubelet.sh

sudo kubeadm init --ignore-preflight-errors=all --cri-socket /etc/firecracker-containerd/fccd-cri.sock --pod-network-cidr=192.168.0.0/16

mkdir -p $HOME/.kube
sudo cp -i /etc/kubernetes/admin.conf $HOME/.kube/config
sudo chown $(id -u):$(id -g) $HOME/.kube/config

# Untaint master (allow pods to be scheduled on master) 
kubectl taint nodes --all node-role.kubernetes.io/master-

$DIR/setup_master_node.sh
