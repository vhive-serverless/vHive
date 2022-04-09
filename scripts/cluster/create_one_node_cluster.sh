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
set -x
DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null 2>&1 && pwd )"
ROOT="$( cd $DIR && cd .. && cd .. && pwd)"

STOCK_CONTAINERD=$1

# Create kubelet service
$DIR/setup_worker_kubelet.sh $STOCK_CONTAINERD

if [ "$STOCK_CONTAINERD" == "stock-only" ]; then
    CRI_SOCK="/run/containerd/containerd.sock"
else
    CRI_SOCK="/etc/vhive-cri/vhive-cri.sock"
fi

sudo kubeadm init --ignore-preflight-errors=all --cri-socket $CRI_SOCK --pod-network-cidr=192.168.0.0/14

mkdir -p $HOME/.kube
sudo cp -i /etc/kubernetes/admin.conf $HOME/.kube/config
sudo chown $(id -u):$(id -g) $HOME/.kube/config

# if the user is root, export KUBECONFIG as $HOME is different for root user and /etc is readable
if [ "$EUID" -eq 0 ]; then
    export KUBECONFIG=/etc/kubernetes/admin.conf
fi


# Untaint master (allow pods to be scheduled on master) 
kubectl taint nodes --all node-role.kubernetes.io/master-

$DIR/setup_master_node.sh $STOCK_CONTAINERD
