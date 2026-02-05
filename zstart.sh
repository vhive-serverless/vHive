API_SOCKET="/tmp/firecracker.socket"

# Remove API unix socket
sudo rm -f $API_SOCKET

# Remove any leftover vsock sockets from previous runs
sudo rm -f /tmp/v.sock /tmp/*.vsock 2>/dev/null || true

LOGFILE="./firecracker.log"
sudo rm -f $LOGFILE

# Run firecracker (installed by vHive setup at /usr/local/bin/firecracker)
if ! command -v firecracker &> /dev/null; then
    echo "ERROR: firecracker binary not found. Run setup_node first."
    exit 1
fi

sudo firecracker --api-sock "${API_SOCKET}" --enable-pci