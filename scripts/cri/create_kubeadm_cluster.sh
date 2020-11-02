#!/bin/bash
# Create kubelet service
cat <<EOF > /etc/systemd/system/kubelet.service.d/0-containerd.conf
[Service]                                                 
Environment="KUBELET_EXTRA_ARGS=--container-runtime=remote --runtime-request-timeout=15m --container-runtime-endpoint=unix:///users/plamenpp/fccd-cri.sock"
EOF
systemctl daemon-reload

kubeadm init --ignore-preflight-errors=all --cri-socket /users/plamenpp/fccd-cri.sock --pod-network-cidr=192.168.0.0/16

# Install Calico network add-on
kubectl apply -f https://docs.projectcalico.org/manifests/calico.yaml

# Untaint master (allow pods to be scheduled on master) 
kubectl taint nodes --all node-role.kubernetes.io/master-


# Install KNative in the cluster
kubectl apply --filename https://github.com/knative/serving/releases/download/v0.17.0/serving-crds.yaml
kubectl apply --filename https://github.com/knative/serving/releases/download/v0.17.0/serving-core.yaml

# Configure network
kubectl apply --filename https://raw.githubusercontent.com/Kong/kubernetes-ingress-controller/0.9.x/deploy/single/all-in-one-dbless.yaml
kubectl patch configmap/config-network \
  --namespace knative-serving \
  --type merge \
  --patch '{"data":{"ingress.class":"kong"}}'

# Path load balancer
PUBLIC_IP=$(curl ifconfig.me)
kubectl patch svc kong-proxy -n kong -p '{"spec": {"type": "LoadBalancer", "externalIPs":["'${PUBLIC_IP}'"]}}'
