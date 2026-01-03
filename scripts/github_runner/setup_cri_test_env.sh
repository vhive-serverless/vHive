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

# KUBECONFIG=/etc/kubernetes/admin.conf sudo $VHIVE_ROOT/scripts/setup_zipkin.sh
$VHIVE_ROOT/scripts/setup_tool -vhive-repo-dir $VHIVE_ROOT setup_zipkin

# FIXME (gh-709)
#source etc/profile && go run $VHIVE_ROOT/examples/registry/populate_registry.go -imageFile $VHIVE_ROOT/examples/registry/images.txt

# Pre-deployment diagnostics
echo "=== Pre-deployment Diagnostics ==="
echo "Checking vHive orchestrator status..."
ps aux | grep "./vhive" | grep -v grep || echo "WARNING: vHive orchestrator not running!"

echo "Checking CRI proxy socket..."
ls -la /etc/vhive-cri/vhive-cri.sock || echo "WARNING: CRI proxy socket not found!"

echo "Checking kubelet CRI socket configuration..."
ps aux | grep kubelet | grep cri-socket || echo "WARNING: kubelet not found or no cri-socket flag!"

echo "Checking existing pods in default namespace..."
sudo KUBECONFIG=/etc/kubernetes/admin.conf kubectl get pods -n default

echo "=== Deploying Services ==="
sudo KUBECONFIG=/etc/kubernetes/admin.conf kn service apply helloworld -f $VHIVE_ROOT/configs/knative_workloads/$SANDBOX/helloworld.yaml &
HELLOWORLD_PID=$!

# Wait a bit and check pod status
sleep 10s

echo "=== Post-deployment Diagnostics ==="
echo "Listing all pods in default namespace..."
sudo KUBECONFIG=/etc/kubernetes/admin.conf kubectl get pods -n default -o wide

echo "Getting pod events..."
sudo KUBECONFIG=/etc/kubernetes/admin.conf kubectl get events -n default --sort-by='.lastTimestamp' | tail -20

# Get helloworld pod name
HELLOWORLD_POD=$(sudo KUBECONFIG=/etc/kubernetes/admin.conf kubectl get pods -n default -l serving.knative.dev/service=helloworld -o jsonpath='{.items[0].metadata.name}' 2>/dev/null)

if [ -n "$HELLOWORLD_POD" ]; then
    echo "Helloworld pod found: $HELLOWORLD_POD"
    
    echo "Pod status:"
    sudo KUBECONFIG=/etc/kubernetes/admin.conf kubectl get pod "$HELLOWORLD_POD" -n default -o yaml | grep -A 20 "status:"
    
    echo "Pod events:"
    sudo KUBECONFIG=/etc/kubernetes/admin.conf kubectl describe pod "$HELLOWORLD_POD" -n default | grep -A 50 "Events:"
    
    echo "Checking if containers are ready..."
    sudo KUBECONFIG=/etc/kubernetes/admin.conf kubectl get pod "$HELLOWORLD_POD" -n default -o jsonpath='{.status.containerStatuses}' | jq .
    
    echo "Getting container logs (if available)..."
    sudo KUBECONFIG=/etc/kubernetes/admin.conf kubectl logs "$HELLOWORLD_POD" -n default --all-containers=true --prefix=true || echo "Logs not available yet"
else
    echo "ERROR: No helloworld pod found!"
fi

# Wait for background kn command to finish
wait $HELLOWORLD_PID

echo "Deploying remaining services..."
sudo KUBECONFIG=/etc/kubernetes/admin.conf kn service apply helloworldserial -f $VHIVE_ROOT/configs/knative_workloads/$SANDBOX/helloworldSerial.yaml
sudo KUBECONFIG=/etc/kubernetes/admin.conf kn service apply pyaes -f $VHIVE_ROOT/configs/knative_workloads/$SANDBOX/pyaes.yaml
sleep 30s
