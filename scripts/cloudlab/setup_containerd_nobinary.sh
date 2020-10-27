#!/bin/bash

set -e

DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null 2>&1 && pwd )"


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

popd >> /dev/null