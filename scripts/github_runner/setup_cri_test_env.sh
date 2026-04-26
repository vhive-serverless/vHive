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
KUBECTL=(sudo KUBECONFIG=/etc/kubernetes/admin.conf kubectl)
KN=(sudo KUBECONFIG=/etc/kubernetes/admin.conf kn)

wait_for_cluster_readiness() {
    echo "Waiting for Kubernetes, networking, and Knative control-plane readiness..."

    "${KUBECTL[@]}" wait --for=condition=Ready nodes --all --timeout=600s

    "${KUBECTL[@]}" -n kube-system wait deploy/coredns deploy/calico-kube-controllers \
        --for=condition=Available --timeout=600s
    "${KUBECTL[@]}" -n kube-system rollout status daemonset/calico-node --timeout=600s

    "${KUBECTL[@]}" -n metallb-system wait deploy/controller \
        --for=condition=Available --timeout=600s
    "${KUBECTL[@]}" -n metallb-system rollout status daemonset/speaker --timeout=600s

    "${KUBECTL[@]}" -n istio-system wait deploy --all \
        --for=condition=Available --timeout=600s
    "${KUBECTL[@]}" -n knative-serving wait deploy --all \
        --for=condition=Available --timeout=600s
    "${KUBECTL[@]}" -n knative-eventing wait deploy --all \
        --for=condition=Available --timeout=600s
    "${KUBECTL[@]}" -n knative-eventing rollout status statefulset/request-reply --timeout=600s

    "${KUBECTL[@]}" -n registry wait pod -l app=registry \
        --for=condition=Ready --timeout=600s
    "${KUBECTL[@]}" -n registry wait pod -l app=registry-etc-hosts-update \
        --for=condition=Ready --timeout=600s
}

wait_for_zipkin() {
    echo "Waiting for Zipkin readiness..."
    "${KUBECTL[@]}" -n istio-system wait deploy/zipkin \
        --for=condition=Available --timeout=600s
}

check_ghcr_from_cluster() {
    echo "Checking in-cluster connectivity to ghcr.io..."

    "${KUBECTL[@]}" -n default delete pod ghcr-connectivity --ignore-not-found
    "${KUBECTL[@]}" -n default run ghcr-connectivity \
        --restart=Never \
        --image=curlimages/curl:8.11.1 \
        --command -- sh -c 'code="$(curl -sS -o /dev/null -w "%{http_code}" --connect-timeout 10 --max-time 30 https://ghcr.io/v2/)"; echo "ghcr.io returned ${code}"; [ "${code}" = "200" ] || [ "${code}" = "401" ]'

    if ! "${KUBECTL[@]}" -n default wait pod/ghcr-connectivity \
        --for=jsonpath='{.status.phase}'=Succeeded --timeout=180s; then
        "${KUBECTL[@]}" -n default describe pod/ghcr-connectivity || true
        "${KUBECTL[@]}" -n default logs pod/ghcr-connectivity || true
        "${KUBECTL[@]}" -n default delete pod ghcr-connectivity --ignore-not-found
        exit 1
    fi

    "${KUBECTL[@]}" -n default logs pod/ghcr-connectivity
    "${KUBECTL[@]}" -n default delete pod ghcr-connectivity --ignore-not-found
}

$VHIVE_ROOT/scripts/setup_tool -vhive-repo-dir $VHIVE_ROOT start_onenode_vhive_cluster $SANDBOX
# $VHIVE_ROOT/scripts/cloudlab/start_onenode_vhive_cluster.sh "$SANDBOX"
wait_for_cluster_readiness

# KUBECONFIG=/etc/kubernetes/admin.conf sudo $VHIVE_ROOT/scripts/setup_zipkin.sh
$VHIVE_ROOT/scripts/setup_tool -vhive-repo-dir $VHIVE_ROOT setup_zipkin
wait_for_zipkin

# FIXME (gh-709)
#source etc/profile && go run $VHIVE_ROOT/examples/registry/populate_registry.go -imageFile $VHIVE_ROOT/examples/registry/images.txt

check_ghcr_from_cluster

"${KN[@]}" service apply helloworld -f $VHIVE_ROOT/configs/knative_workloads/$SANDBOX/helloworld.yaml
"${KN[@]}" service apply helloworldserial -f $VHIVE_ROOT/configs/knative_workloads/$SANDBOX/helloworldSerial.yaml
"${KN[@]}" service apply pyaes -f $VHIVE_ROOT/configs/knative_workloads/$SANDBOX/pyaes.yaml

"${KUBECTL[@]}" wait ksvc/helloworld ksvc/helloworldserial ksvc/pyaes \
    --for=condition=Ready --timeout=600s

if [ "$SANDBOX" == "gvisor" ]; then
    "${KUBECTL[@]}" get runtimeclass gvisor
    FEATURE=$("${KUBECTL[@]}" get cm config-features -n knative-serving -o go-template='{{ index .data "kubernetes.podspec-runtimeclassname" }}')
    if [ "$FEATURE" != "enabled" ]; then
        echo "Knative RuntimeClass support is not enabled"
        exit 1
    fi

    POD_RUNTIME_CLASSES=$("${KUBECTL[@]}" get pods -n default -l 'serving.knative.dev/service in (helloworld,helloworldserial,pyaes)' -o jsonpath='{range .items[*]}{.spec.runtimeClassName}{"\n"}{end}')
    if [ -z "$POD_RUNTIME_CLASSES" ]; then
        echo "No gVisor test pods found"
        exit 1
    fi
    if echo "$POD_RUNTIME_CLASSES" | grep -v '^gvisor$'; then
        echo "Expected all gVisor test pods to use runtimeClassName=gvisor"
        exit 1
    fi
fi
