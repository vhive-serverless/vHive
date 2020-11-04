#!/bin/bash
#set -e

DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null 2>&1 && pwd )"

sudo apt-get update 2>&1

sudo apt-get -y install \
	apt-transport-https \
	ca-certificates \
	curl \
	gnupg-agent \
	dmsetup \
  git-lfs \
	software-properties-common 2>&1


# enable KVM
sudo setfacl -m u:${USER}:rw /dev/kvm
sudo modprobe msr

git lfs pull
BINS=./bin
DST=/usr/local/bin


sudo mkdir -p /var/lib/firecracker-containerd/runtime
sudo mkdir -p /etc/firecracker-containerd
sudo mkdir -p /var/lib/firecracker-containerd/containerd
sudo mkdir -p /etc/containerd

sudo mkdir -p /var/lib/firecracker-containerd

pushd $BINS

sudo cp firecracker $DST
sudo cp jailer $DST
sudo cp containerd-shim-aws-firecracker $DST
sudo cp firecracker-containerd $DST
sudo cp firecracker-ctr $DST
sudo cp default-rootfs.img /var/lib/firecracker-containerd/runtime/
popd 

echo Configure firecracker-containerd
curl -fsSL -o hello-vmlinux.bin https://s3.amazonaws.com/spec.ccfc.min/img/hello/kernel/hello-vmlinux.bin
sudo mv ./hello-vmlinux.bin /var/lib/firecracker-containerd/runtime/default-vmlinux.bin

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

# Setup device mapper thin pool
./scripts/create_devmapper.sh

sudo tee /etc/containerd/firecracker-runtime.json <<EOF
{
  "firecracker_binary_path": "/usr/local/bin/firecracker",
  "cpu_template": "T2",
  "log_fifo": "fc-logs.fifo",
  "log_levels": ["debug"],
  "metrics_fifo": "fc-metrics.fifo",
  "kernel_args": "console=ttyS0 noapic reboot=k panic=1 pci=off nomodules ro systemd.journald.forward_to_console systemd.unit=firecracker.target init=/sbin/overlay-init tsc=reliable quiet 8250.nr_uarts=0 ipv6.disable=1"
}
EOF