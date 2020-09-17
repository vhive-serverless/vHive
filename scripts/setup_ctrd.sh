#!/bin/bash
#set -e

sudo apt-get update 2>&1

sudo apt-get -y install \
	apt-transport-https \
	ca-certificates \
	curl \
	gnupg-agent \
	dmsetup \
	software-properties-common 2>&1

curl -fsSL https://download.docker.com/linux/ubuntu/gpg | sudo apt-key add -

sudo apt-key fingerprint 0EBFCD88

sudo add-apt-repository \
   "deb [arch=amd64] https://download.docker.com/linux/ubuntu \
   $(lsb_release -cs) \
   stable"

sudo apt-get update

sudo apt-get -y install docker-ce docker-ce-cli containerd.io

# enable KVM
sudo setfacl -m u:${USER}:rw /dev/kvm
sudo modprobe msr


# firecracker-containerd and thinpool for devmapper

err=""; \
    [ "$(uname) $(uname -m)" = "Linux x86_64" ] \
    || err="ERROR: your system is not Linux x86_64."; \
    [ -r /dev/kvm ] && [ -w /dev/kvm ] \
    || err="$err\nERROR: /dev/kvm is innaccessible."; \
    (( $(uname -r | cut -d. -f1)*1000 + $(uname -r | cut -d. -f2) >= 4014 )) \
    || err="$err\nERROR: your kernel version ($(uname -r)) is too old."; \
    dmesg | grep -i "hypervisor detected" \
    && echo "WARNING: you are running in a virtual machine. Firecracker is not well tested under nested virtualization."; \
    [ -z "$err" ] && echo "Your system looks ready for Firecracker!" || echo -e "$err"

git clone -b kill_shim --recurse-submodules https://github.com/ustiugov/firecracker-containerd

echo Configure firecracker-containerd
pushd firecracker-containerd > /dev/null
curl -fsSL -o hello-vmlinux.bin https://s3.amazonaws.com/spec.ccfc.min/img/hello/kernel/hello-vmlinux.bin
make all
make firecracker
make image
sudo make install install-firecracker demo-network
sudo mkdir -p /var/lib/firecracker-containerd/runtime
sudo cp tools/image-builder/rootfs.img /var/lib/firecracker-containerd/runtime/default-rootfs.img
sudo mkdir -p /etc/firecracker-containerd
sudo mkdir -p /var/lib/firecracker-containerd/containerd

sudo mkdir -p /var/lib/firecracker-containerd
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
../scripts/create_devmapper.sh

sudo mkdir -p /var/lib/firecracker-containerd/runtime
sudo mv ./hello-vmlinux.bin /var/lib/firecracker-containerd/runtime/default-vmlinux.bin
sudo mkdir -p /etc/containerd
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
popd > /dev/null



