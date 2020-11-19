#!/bin/bash
sudo apt-get update >> /dev/null

sudo apt-get -y install btrfs-tools pkg-config libseccomp-dev unzip tar libseccomp2 socat util-linux apt-transport-https curl ipvsadm >> /dev/null

wget -c https://github.com/google/protobuf/releases/download/v3.11.4/protoc-3.11.4-linux-x86_64.zip
sudo unzip protoc-3.11.4-linux-x86_64.zip -d /usr/local

# Build and install runc and containerd
GOGITHUB=${HOME}/go/src/github.com/
RUNC_ROOT=${GOGITHUB}/opencontainers/runc
CONTAINERD_ROOT=${GOGITHUB}/containerd/containerd
mkdir -p $RUNC_ROOT
mkdir -p $CONTAINERD_ROOT

git clone https://github.com/opencontainers/runc.git $RUNC_ROOT
git clone -b cri_logging https://github.com/plamenmpetrov/containerd.git $CONTAINERD_ROOT

cd $RUNC_ROOT
make && sudo make install

cd $CONTAINERD_ROOT
make && sudo make install

containerd --version || echo "failed to build containerd"


# Install k8s
curl -s https://packages.cloud.google.com/apt/doc/apt-key.gpg | sudo apt-key add -
sudo sh -c "echo 'deb http://apt.kubernetes.io/ kubernetes-xenial main' > /etc/apt/sources.list.d/kubernetes.list"
sudo apt update >> /dev/null
sudo apt -y install cri-tools ebtables ethtool kubeadm kubectl kubelet kubernetes-cni

# Install knative CLI
git clone https://github.com/knative/client.git $HOME/client
cd $HOME/client
hack/build.sh -f
sudo mv kn /usr/local/bin


# Necessary for containerd as container runtime but not docker
sudo modprobe overlay
sudo modprobe br_netfilter

# Set up required sysctl params, these persist across reboots.
sudo tee /etc/sysctl.d/99-kubernetes-cri.conf <<EOF
net.bridge.bridge-nf-call-iptables  = 1
net.ipv4.ip_forward                 = 1
net.bridge.bridge-nf-call-ip6tables = 1
EOF

sudo sysctl --system
# ---------------------------------------------------------

sudo swapoff -a
sudo sysctl net.ipv4.ip_forward=1
