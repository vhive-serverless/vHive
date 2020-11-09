#!/bin/bash
# Create kubelet service
cat <<EOF > /etc/systemd/system/kubelet.service.d/0-containerd.conf
[Service]                                                 
Environment="KUBELET_EXTRA_ARGS=--container-runtime=remote --runtime-request-timeout=15m --container-runtime-endpoint=unix:///run/containerd/containerd.sock"
EOF
systemctl daemon-reload

kubeadm init --ignore-preflight-errors=all --cri-socket /run/containerd/containerd.sock --pod-network-cidr=192.168.0.0/16

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

# Untaint master (allow pods to be scheduled on master) 
# NOTE: (Plamen) schedule all dataplane pods on worker nodes
# kubectl taint nodes --all node-role.kubernetes.io/master-

# Install and configure MetalLB
kubectl get configmap kube-proxy -n kube-system -o yaml | \
sed -e "s/strictARP: false/strictARP: true/" | \
kubectl apply -f - -n kube-system

kubectl apply -f https://raw.githubusercontent.com/metallb/metallb/v0.9.4/manifests/namespace.yaml
kubectl apply -f https://raw.githubusercontent.com/metallb/metallb/v0.9.4/manifests/metallb.yaml
kubectl create secret generic -n metallb-system memberlist --from-literal=secretkey="$(openssl rand -base64 128)"

tee metallb-configmap.yaml <<EOF
apiVersion: v1
kind: ConfigMap
metadata:
  namespace: metallb-system
  name: config
data:
  config: |
    address-pools:
    - name: default
      protocol: layer2
      addresses:
      - 192.168.1.240-192.168.1.250
EOF
kubectl apply -f metallb-configmap.yaml

# istio
curl -L https://istio.io/downloadIstio | ISTIO_VERSION=1.6.11 TARGET_ARCH=x86_64 sh -
cat << EOF > ./istio-minimal-operator.yaml
apiVersion: install.istio.io/v1alpha1
kind: IstioOperator
spec:
  values:
    global:
      proxy:
        autoInject: disabled
      useMCP: false
      # The third-party-jwt is not enabled on all k8s.
      # See: https://istio.io/docs/ops/best-practices/security/#configure-third-party-service-account-tokens
      jwtPolicy: first-party-jwt

  addonComponents:
    pilot:
      enabled: true
    prometheus:
      enabled: false

  components:
    ingressGateways:
      - name: istio-ingressgateway
        enabled: true
      - name: cluster-local-gateway
        enabled: true
        label:
          istio: cluster-local-gateway
          app: cluster-local-gateway
        k8s:
          service:
            type: ClusterIP
            ports:
            - port: 15020
              name: status-port
            - port: 80
              name: http2
            - port: 443
              name: https
EOF

./istio-1.6.11/bin/istioctl manifest apply -f istio-minimal-operator.yaml


# Install KNative in the cluster
kubectl apply --filename https://github.com/knative/serving/releases/download/v0.17.0/serving-crds.yaml
kubectl apply --filename https://github.com/knative/serving/releases/download/v0.17.0/serving-core.yaml

# magic DNS
kubectl apply --filename https://github.com/knative/serving/releases/download/v0.18.0/serving-default-domain.yaml

kubectl apply --filename https://github.com/knative/net-istio/releases/download/v0.18.0/release.yaml
kubectl --namespace istio-system get service istio-ingressgateway
