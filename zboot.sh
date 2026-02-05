TAP_DEV="tap0"
TAP_IP="10.10.1.1"
MASK_SHORT="/30"

# Setup network interface
sudo ip link del "$TAP_DEV" 2> /dev/null || true
sudo ip tuntap add dev "$TAP_DEV" mode tap
sudo ip addr add "${TAP_IP}${MASK_SHORT}" dev "$TAP_DEV"
sudo ip link set dev "$TAP_DEV" up

# Enable ip forwarding
sudo sh -c "echo 1 > /proc/sys/net/ipv4/ip_forward"
sudo iptables -P FORWARD ACCEPT

# This tries to determine the name of the host network interface to forward
# VM's outbound network traffic through. If outbound traffic doesn't work,
# double check this returns the correct interface!
HOST_IFACE=$(ip -j route list default |jq -r '.[0].dev')

# Set up microVM internet access
sudo iptables -t nat -D POSTROUTING -o "$HOST_IFACE" -j MASQUERADE || true
sudo iptables -t nat -A POSTROUTING -o "$HOST_IFACE" -j MASQUERADE

API_SOCKET="/tmp/firecracker.socket"
LOGFILE="$HOME/firecracker.log"

# Set machine configuration
sudo curl -X PUT --unix-socket "${API_SOCKET}" \
    --data "{
        \"vcpu_count\": 2,
        \"mem_size_mib\": 1024,
        \"track_dirty_pages\": false
    }" \
    "http://localhost/machine-config"


# # Create log file
# touch $LOGFILE

# Set log file
sudo curl -X PUT --unix-socket "${API_SOCKET}" \
    --data "{
        \"log_path\": \"${LOGFILE}\",
        \"level\": \"Debug\",
        \"show_level\": true,
        \"show_log_origin\": true
    }" \
    "http://localhost/logger"


VSOCK_DIR="/tmp/v.sock"
sudo rm -f ${VSOCK_DIR}
sudo curl --unix-socket "${API_SOCKET}" -i \
    -X PUT 'http://localhost/vsock' \
    -H 'Accept: application/json' \
    -H 'Content-Type: application/json' \
    -d '{
        "guest_cid": 3,
        "uds_path": "'${VSOCK_DIR}'"
    }'

# vHive kernel path (setup_node copies vmlinux-6.1.141 here)
KERNEL="/var/lib/firecracker-containerd/runtime/hello-vmlinux.bin"
if [ ! -f "$KERNEL" ]; then
    echo "ERROR: Kernel not found at $KERNEL. Run setup_node first."
    exit 1
fi

KERNEL_BOOT_ARGS="console=ttyS0 reboot=k panic=1 i8042.nokbd i8042.noaux 8250.nr_uarts=0 ipv6.disable=1 overlay_root=ram init=/sbin/overlay-init iomem=relaxed"

ARCH=$(uname -m)

if [ ${ARCH} = "aarch64" ]; then
    KERNEL_BOOT_ARGS="keep_bootcon ${KERNEL_BOOT_ARGS}"
fi

# Set boot source
sudo curl -X PUT --unix-socket "${API_SOCKET}" \
    --data "{
        \"kernel_image_path\": \"${KERNEL}\",
        \"boot_args\": \"${KERNEL_BOOT_ARGS}\"
    }" \
    "http://localhost/boot-source"

# vHive rootfs path (setup_node copies default-rootfs.img here)
ROOTFS="/var/lib/firecracker-containerd/runtime/default-rootfs.img"
if [ ! -f "$ROOTFS" ]; then
    echo "ERROR: Rootfs not found at $ROOTFS. Run setup_node first."
    exit 1
fi

# Set rootfs
sudo curl -X PUT --unix-socket "${API_SOCKET}" \
    --data "{
        \"drive_id\": \"rootfs\",
        \"path_on_host\": \"${ROOTFS}\",
        \"is_root_device\": true,
        \"is_read_only\": false
    }" \
    "http://localhost/drives/rootfs"

# The IP address of a guest is derived from its MAC address with
# `fcnet-setup.sh`, this has been pre-configured in the guest rootfs. It is
# important that `TAP_IP` and `FC_MAC` match this.
FC_MAC="06:00:0A:0A:01:02"

# Set network interface
sudo curl -X PUT --unix-socket "${API_SOCKET}" \
    --data "{
        \"iface_id\": \"net1\",
        \"guest_mac\": \"$FC_MAC\",
        \"host_dev_name\": \"$TAP_DEV\"
    }" \
    "http://localhost/network-interfaces/net1"

sleep 2s

sudo curl -X PUT --unix-socket "${API_SOCKET}" \
    --data "{
        \"action_type\": \"InstanceStart\"
    }" \
    "http://localhost/actions"


# ip route add default via 10.10.1.1 dev eth0