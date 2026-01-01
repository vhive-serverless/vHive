#!/bin/bash

# MIT License
#
# Copyright (c) 2020 Dmitrii Ustiugov, Shyam Jesalpura and EASE lab
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

set -Eeuo pipefail

cd "$( dirname "${BASH_SOURCE[0]}" )"

if (( $# != 1)); then
    echo "Invalid number of parameters"
    echo "USAGE: setup_cri_test_env.sh <sandbox>"
    exit 1
fi

SANDBOX=$1
VHIVE_ROOT="$(git rev-parse --show-toplevel)"

$VHIVE_ROOT/scripts/setup_tool -vhive-repo-dir $VHIVE_ROOT start_onenode_vhive_cluster $SANDBOX
# $VHIVE_ROOT/scripts/cloudlab/start_onenode_vhive_cluster.sh "$SANDBOX"
sleep 30s

# Fix CoreDNS for external DNS resolution (must run after K8s cluster is created)
echo "Patching CoreDNS to use host DNS resolver..."
sudo KUBECONFIG=/etc/kubernetes/admin.conf kubectl get configmap coredns -n kube-system -o yaml > /tmp/coredns-backup.yaml

# Get host DNS server - try multiple detection methods
HOST_DNS=""

# Try resolvectl (newer systemd)
if command -v resolvectl &> /dev/null; then
  HOST_DNS=$(resolvectl status 2>/dev/null | grep "DNS Servers" | head -n1 | awk '{print $3}')
fi

# Try reading systemd's resolved.conf
if [ -z "$HOST_DNS" ] && [ -f /run/systemd/resolve/resolv.conf ]; then
  HOST_DNS=$(grep "^nameserver" /run/systemd/resolve/resolv.conf | head -n1 | awk '{print $2}')
fi

# Try reading /etc/resolv.conf
if [ -z "$HOST_DNS" ]; then
  HOST_DNS=$(grep "^nameserver" /etc/resolv.conf | head -n1 | awk '{print $2}')
fi

# Default to Google DNS if nothing worked
if [ -z "$HOST_DNS" ]; then
  HOST_DNS="8.8.8.8"
  echo "Could not detect host DNS, using Google DNS: $HOST_DNS"
else
  echo "Detected host DNS: $HOST_DNS"
fi

# Patch CoreDNS ConfigMap to forward to host DNS
sudo KUBECONFIG=/etc/kubernetes/admin.conf kubectl get configmap coredns -n kube-system -o json | \
  jq --arg dns "$HOST_DNS" '.data.Corefile |= gsub("forward . /etc/resolv.conf"; "forward . " + $dns)' | \
  sudo KUBECONFIG=/etc/kubernetes/admin.conf kubectl apply -f -

echo "Restarting CoreDNS pods..."
sudo KUBECONFIG=/etc/kubernetes/admin.conf kubectl rollout restart deployment/coredns -n kube-system
sudo KUBECONFIG=/etc/kubernetes/admin.conf kubectl rollout status deployment/coredns -n kube-system --timeout=60s

echo "Waiting for DNS to propagate..."
sleep 10

# KUBECONFIG=/etc/kubernetes/admin.conf sudo $VHIVE_ROOT/scripts/setup_zipkin.sh
$VHIVE_ROOT/scripts/setup_tool -vhive-repo-dir $VHIVE_ROOT setup_zipkin

# FIXME (gh-709)
#source etc/profile && go run $VHIVE_ROOT/examples/registry/populate_registry.go -imageFile $VHIVE_ROOT/examples/registry/images.txt

sudo KUBECONFIG=/etc/kubernetes/admin.conf kn service apply helloworld -f $VHIVE_ROOT/configs/knative_workloads/$SANDBOX/helloworld.yaml
sudo KUBECONFIG=/etc/kubernetes/admin.conf kn service apply helloworldserial -f $VHIVE_ROOT/configs/knative_workloads/$SANDBOX/helloworldSerial.yaml
sudo KUBECONFIG=/etc/kubernetes/admin.conf kn service apply pyaes -f $VHIVE_ROOT/configs/knative_workloads/$SANDBOX/pyaes.yaml
sleep 30s
