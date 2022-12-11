#!/usr/bin/env bash

set -eo pipefail

kindVersion=$(kind version);
K8S_VERSION=${k8sVersion:-v1.23.4@sha256:0e34f0d0fd448aa2f2819cfd74e99fe5793a6e4938b328f657c8e3f81ee0dfb9}
KIND_BASE=${KIND_BASE:-kindest/node}
CLUSTER_NAME=${KIND_CLUSTER_NAME:-knative}

echo "KinD version is ${kindVersion}"
if [[ ! $kindVersion =~ "${KIND_VERSION}." ]]; then
  echo "WARNING: Please make sure you are using KinD version ${KIND_VERSION}.x, download from https://github.com/kubernetes-sigs/kind/releases"
fi

REPLY=continue
KIND_EXIST="$(kind get clusters -q | grep ${CLUSTER_NAME} || true)"
if [[ ${KIND_EXIST} ]] ; then
  echo "WARNING: Knative Cluster kind-${CLUSTER_NAME} already installed -> delete"
  kind delete cluster --name ${CLUSTER_NAME}
fi

echo "Using image ${KIND_BASE}:${K8S_VERSION}"
KIND_CLUSTER=$(mktemp)
cat <<EOF | kind create cluster --name ${CLUSTER_NAME} --wait 120s --config=-
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
- role: control-plane
  image: ${KIND_BASE}:${K8S_VERSION}
  extraPortMappings:
  - containerPort: 31080 # expose port 31380 of the node to port 80 on the host, later to be use by kourier or contour ingress
    listenAddress: 127.0.0.1
    hostPort: 80
EOF