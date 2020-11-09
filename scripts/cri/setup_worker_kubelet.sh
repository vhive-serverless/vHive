#!/bin/bash
# Create kubelet service
cat <<EOF > /etc/systemd/system/kubelet.service.d/0-containerd.conf
[Service]                                                 
Environment="KUBELET_EXTRA_ARGS=--container-runtime=remote --runtime-request-timeout=15m --container-runtime-endpoint=unix:///etc/firecracker-containerd/fccd-cri.sock"
EOF
systemctl daemon-reload
