#!/bin/bash
DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null 2>&1 && pwd )"
ROOT="$( cd $DIR && cd .. && cd .. && pwd)"

# Install Calico network add-on
curl https://docs.projectcalico.org/manifests/canal.yaml -O
kubectl apply -f canal.yaml

# Install and configure MetalLB
kubectl get configmap kube-proxy -n kube-system -o yaml | \
sed -e "s/strictARP: false/strictARP: true/" | \
kubectl apply -f - -n kube-system

kubectl apply -f https://raw.githubusercontent.com/metallb/metallb/v0.9.4/manifests/namespace.yaml
kubectl apply -f https://raw.githubusercontent.com/metallb/metallb/v0.9.4/manifests/metallb.yaml
kubectl create secret generic -n metallb-system memberlist --from-literal=secretkey="$(openssl rand -base64 128)"
kubectl apply -f $ROOT/configs/metallb/metallb-configmap.yaml

# istio
curl -L https://istio.io/downloadIstio | ISTIO_VERSION=1.6.11 TARGET_ARCH=x86_64 sh -
./istio-1.6.11/bin/istioctl manifest apply -f $ROOT/configs/istio/istio-minimal-operator.yaml


# Install KNative in the cluster
kubectl apply --filename $ROOT/configs/knative_yamls/serving-crds.yaml
kubectl apply --filename $ROOT/configs/knative_yamls/serving-core.yaml

# magic DNS
kubectl apply --filename $ROOT/configs/knative_yamls/serving-default-domain.yaml

kubectl apply --filename https://github.com/knative/net-istio/releases/download/v0.19.0/release.yaml
kubectl --namespace istio-system get service istio-ingressgateway
