#!/bin/bash
sudo apt-get update 1>/dev/null

DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null 2>&1 && pwd )"

sudo apt-get install -y btrfs-tools pkg-config libseccomp-dev unzip tar libseccomp2 socat util-linux apt-transport-https curl ipvsadm git-lfs 1>/dev/null

pushd $DIR
git lfs pull 1>/dev/null
popd

wget -c https://github.com/google/protobuf/releases/download/v3.11.4/protoc-3.11.4-linux-x86_64.zip 1>/dev/null
sudo unzip protoc-3.11.4-linux-x86_64.zip -d /usr/local 1>/dev/null


export KUBECONFIG=/etc/kubernetes/admin.conf
sudo echo 'export KUBECONFIG=/etc/kubernetes/admin.conf' >> /etc/profile

# Build and install runc and containerd
GOGITHUB=${HOME}/go/src/github.com/
RUNC_ROOT=${GOGITHUB}/opencontainers/runc
CONTAINERD_ROOT=${GOGITHUB}/containerd/containerd
mkdir -p $RUNC_ROOT
mkdir -p $CONTAINERD_ROOT

git clone https://github.com/opencontainers/runc.git $RUNC_ROOT 1>/dev/null
git clone -b cri_logging https://github.com/plamenmpetrov/containerd.git $CONTAINERD_ROOT 1>/dev/null

cd $RUNC_ROOT
sudo env PATH=$PATH make && sudo env PATH=$PATH make install 1>/dev/null

cd $CONTAINERD_ROOT
sudo env PATH=$PATH make && sudo env PATH=$PATH make install 1>/dev/null

containerd --version || echo "failed to build containerd"


# Install k8s
curl -s https://packages.cloud.google.com/apt/doc/apt-key.gpg | apt-key add -
sudo env PATH=$PATH echo "deb http://apt.kubernetes.io/ kubernetes-xenial main" > /etc/apt/sources.list.d/kubernetes.list
sudo apt install -y cri-tools ebtables ethtool kubeadm kubectl kubelet kubernetes-cni 1>/dev/null


# Install knative CLI
git clone https://github.com/knative/client.git $HOME/client 1>/dev/null
cd $HOME/client
sudo env PATH=$PATH hack/build.sh -f 1>/dev/null
sudo mv kn /usr/local/bin


# Necessary for containerd as container runtime but not docker
sudo modprobe overlay
sudo modprobe br_netfilter

# Set up required sysctl params, these persist across reboots.
cat <<EOF | sudo tee /etc/sysctl.d/99-kubernetes-cri.conf
net.bridge.bridge-nf-call-iptables  = 1
net.ipv4.ip_forward                 = 1
net.bridge.bridge-nf-call-ip6tables = 1
EOF

sudo sysctl --system
# ---------------------------------------------------------
sudo swapoff -a
sudo sysctl net.ipv4.ip_forward=1


################################
# Setup firecracker-containerd
sudo mkdir -p /var/lib/firecracker-containerd/runtime
sudo curl -fsSL -o /var/lib/firecracker-containerd/runtime/hello-vmlinux.bin https://s3.amazonaws.com/spec.ccfc.min/img/hello/kernel/hello-vmlinux.bin 1>/dev/null

sudo mkdir -p /etc/firecracker-containerd

sudo tee /etc/firecracker-containerd/config.toml <<EOF
disabled_plugins = ["cri"]
root = "/var/lib/firecracker-containerd/containerd"
state = "/run/firecracker-containerd"
[grpc]
  address = "/run/firecracker-containerd/containerd.sock"
[plugins]
  [plugins.devmapper]
    pool_name = "fc-dev-thinpool"
    base_image_size = "10GB"
    root_path = "/var/lib/firecracker-containerd/snapshotter/devmapper"

[debug]
  level = "debug"
EOF

sudo mkdir -p /etc/containerd/

sudo tee /etc/containerd/firecracker-runtime.json <<EOF
{
  "firecracker_binary_path": "/usr/local/bin/firecracker",
  "kernel_image_path": "/var/lib/firecracker-containerd/runtime/hello-vmlinux.bin",
  "kernel_args": "console=ttyS0 noapic reboot=k panic=1 pci=off nomodules ro systemd.journald.forward_to_console systemd.unit=firecracker.target init=/sbin/overlay-init",
  "root_drive": "/var/lib/firecracker-containerd/runtime/default-rootfs.img",
  "cpu_template": "T2",
  "log_levels": ["info"]
}
EOF


cd $DIR
../create_devmapper.sh 1>/dev/null

BINS=../../bin/
DST=/usr/local/bin

pushd $BINS

sudo cp firecracker $DST
sudo cp jailer $DST
sudo cp containerd-shim-aws-firecracker $DST
sudo cp firecracker-containerd $DST
sudo cp firecracker-ctr $DST
sudo cp default-rootfs.img /var/lib/firecracker-containerd/runtime/

popd

sudo env PATH=$PATH containerd 1>/dev/null 2>/dev/null &
sudo PATH=$PATH /usr/local/bin/firecracker-containerd --config /etc/firecracker-containerd/config.toml 1>/dev/null 2>/dev/null &

cd $DIR

cd ../../../
sudo env PATH=$PATH go build ./...

cd $DIR
sudo env PATH=$PATH ./../../fccd-orchestrator &

sudo env PATH=$PATH ./../cri/create_kubeadm_cluster.sh

sleep 2m

sudo env PATH=$PATH kubectl apply -f ../../knative_workloads/helloworld.yaml
sudo env PATH=$PATH kubectl apply -f ../../knative_workloads/pyaes.yaml
sudo env PATH=$PATH kubectl apply -f ../../knative_workloads/rnn_serving.yaml

