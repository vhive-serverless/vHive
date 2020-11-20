#!/bin/bash
DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null 2>&1 && pwd )"
ROOT="$( cd $DIR && cd .. && cd .. && pwd)"

# Create kubelet service
sudo sh -c 'cat <<EOF > /etc/systemd/system/kubelet.service.d/0-containerd.conf
[Service]                                                 
Environment="KUBELET_EXTRA_ARGS=--container-runtime=remote --runtime-request-timeout=15m --container-runtime-endpoint=unix:///run/containerd/containerd.sock"
EOF'
sudo systemctl daemon-reload

sudo kubeadm init --ignore-preflight-errors=all --cri-socket /run/containerd/containerd.sock --pod-network-cidr=192.168.0.0/16

mkdir -p $HOME/.kube
sudo cp -i /etc/kubernetes/admin.conf $HOME/.kube/config
sudo chown $(id -u):$(id -g) $HOME/.kube/config

# Wait until all workers are connected
while true; do
    read -p "All nodes need to be joined in the cluster. Have you joined all nodes? (y/n): " yn
    case $yn in
        [Yy]* ) break;;
        [Nn]* ) continue;;
        * ) echo "Please answer yes or no.";;
    esac
done

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
kubectl apply -f $ROOT/configs/metallb-configmap.yaml

# istio
curl -L https://istio.io/downloadIstio | ISTIO_VERSION=1.6.11 TARGET_ARCH=x86_64 sh -
./istio-1.6.11/bin/istioctl manifest apply -f $ROOT/configs/istio-minimal-operator.yaml


# Install KNative in the cluster
kubectl apply --filename https://github.com/knative/serving/releases/download/v0.17.0/serving-crds.yaml
kubectl apply --filename https://github.com/knative/serving/releases/download/v0.17.0/serving-core.yaml

# magic DNS
kubectl apply --filename https://github.com/knative/serving/releases/download/v0.18.0/serving-default-domain.yaml

kubectl apply --filename https://github.com/knative/net-istio/releases/download/v0.18.0/release.yaml
kubectl --namespace istio-system get service istio-ingressgateway
