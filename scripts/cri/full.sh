#!/bin/bash
sudo apt-get update

DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null 2>&1 && pwd )"

apt-get install -y btrfs-tools pkg-config libseccomp-dev unzip tar libseccomp2 socat util-linux apt-transport-https curl ipvsadm git-lfs

pushd $DIR
git lfs pull
popd

wget -c https://github.com/google/protobuf/releases/download/v3.11.4/protoc-3.11.4-linux-x86_64.zip
sudo unzip protoc-3.11.4-linux-x86_64.zip -d /usr/local

wget https://golang.org/dl/go1.15.2.linux-amd64.tar.gz
tar -C /usr/local -xzf go1.15.2.linux-amd64.tar.gz

export PATH=$PATH:/usr/local/go/bin
echo 'export PATH=$PATH:/usr/local/go/bin' >> /etc/profile

go env -w GOPRIVATE=github.com/ustiugov/*

export KUBECONFIG=/etc/kubernetes/admin.conf
echo 'export KUBECONFIG=/etc/kubernetes/admin.conf' >> /etc/profile

# Build and install runc and containerd
GOGITHUB=${HOME}/go/src/github.com/
RUNC_ROOT=${GOGITHUB}/opencontainers/runc
CONTAINERD_ROOT=${GOGITHUB}/containerd/containerd
mkdir -p $RUNC_ROOT
mkdir -p $CONTAINERD_ROOT

git clone https://github.com/opencontainers/runc.git $RUNC_ROOT
git clone -b cri_logging https://github.com/plamenmpetrov/containerd.git $CONTAINERD_ROOT

cd $RUNC_ROOT
make && make install

cd $CONTAINERD_ROOT
make && make install

containerd --version || echo "failed to build containerd"


# Install k8s
curl -s https://packages.cloud.google.com/apt/doc/apt-key.gpg | apt-key add -
echo "deb http://apt.kubernetes.io/ kubernetes-xenial main" > /etc/apt/sources.list.d/kubernetes.list
apt update
apt install -y cri-tools ebtables ethtool kubeadm kubectl kubelet kubernetes-cni


# Install knative CLI
git clone https://github.com/knative/client.git $HOME/client
cd $HOME/client
hack/build.sh -f
mv kn /usr/local/bin


# Necessary for containerd as container runtime but not docker
modprobe overlay
modprobe br_netfilter

# Set up required sysctl params, these persist across reboots.
cat <<EOF | sudo tee /etc/sysctl.d/99-kubernetes-cri.conf
net.bridge.bridge-nf-call-iptables  = 1
net.ipv4.ip_forward                 = 1
net.bridge.bridge-nf-call-ip6tables = 1
EOF

sudo sysctl --system
# ---------------------------------------------------------
swapoff -a
sysctl net.ipv4.ip_forward=1


################################
# Setup firecracker-containerd
sudo mkdir -p /var/lib/firecracker-containerd/runtime
curl -fsSL -o /var/lib/firecracker-containerd/runtime/hello-vmlinux.bin https://s3.amazonaws.com/spec.ccfc.min/img/hello/kernel/hello-vmlinux.bin

mkdir -p /etc/firecracker-containerd

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

mkdir -p /etc/containerd/

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
../create_devmapper.sh

BINS=../../bin/
DST=/usr/local/bin

pushd $BINS

cp firecracker $DST
cp jailer $DST
cp containerd-shim-aws-firecracker $DST
cp firecracker-containerd $DST
cp firecracker-ctr $DST
cp default-rootfs.img /var/lib/firecracker-containerd/runtime/

popd
