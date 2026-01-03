#!/bin/bash

# Pre-pull images and patch Knative configs with digest-based references
# GitHub runners block port 53 egress from pods, so we must pull images on host
# and use digest-based references to avoid DNS resolution in pods

set -e

if [ $# -ne 1 ]; then
    echo "Usage: $0 <sandbox>"
    echo "  sandbox: gvisor, firecracker, or stock-only"
    exit 1
fi

SANDBOX=$1
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
VHIVE_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

echo "=== Pre-pulling container images on host ==="
sudo ctr -n k8s.io images pull ghcr.io/ease-lab/helloworld:var_workload
sudo ctr -n k8s.io images pull ghcr.io/ease-lab/pyaes:var_workload

echo ""
echo "=== Getting image digests ==="
HELLOWORLD_DIGEST=$(sudo ctr -n k8s.io images ls | grep "ghcr.io/ease-lab/helloworld:var_workload" | awk '{print $3}' | sed 's/sha256://')
PYAES_DIGEST=$(sudo ctr -n k8s.io images ls | grep "ghcr.io/ease-lab/pyaes:var_workload" | awk '{print $3}' | sed 's/sha256://')

if [ -z "$HELLOWORLD_DIGEST" ] || [ -z "$PYAES_DIGEST" ]; then
    echo "ERROR: Failed to get image digests"
    sudo ctr -n k8s.io images ls | grep ghcr.io/ease-lab
    exit 1
fi

echo "Helloworld digest: $HELLOWORLD_DIGEST"
echo "Pyaes digest: $PYAES_DIGEST"

echo ""
echo "=== Patching Knative configs with digest-based references ==="
# Replace image references with digest-based references
for config in "$VHIVE_ROOT/configs/knative_workloads/$SANDBOX/helloworld.yaml" \
              "$VHIVE_ROOT/configs/knative_workloads/$SANDBOX/helloworldSerial.yaml"; do
    if [ -f "$config" ]; then
        echo "Patching $(basename $config)"
        sed -i "s|ghcr.io/ease-lab/helloworld:var_workload|ghcr.io/ease-lab/helloworld@sha256:$HELLOWORLD_DIGEST|g" "$config"
    fi
done

if [ -f "$VHIVE_ROOT/configs/knative_workloads/$SANDBOX/pyaes.yaml" ]; then
    echo "Patching pyaes.yaml"
    sed -i "s|ghcr.io/ease-lab/pyaes:var_workload|ghcr.io/ease-lab/pyaes@sha256:$PYAES_DIGEST|g" \
        "$VHIVE_ROOT/configs/knative_workloads/$SANDBOX/pyaes.yaml"
fi
