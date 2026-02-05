API_SOCKET="/tmp/firecracker.socket"

# Remove API unix socket
sudo rm -f $API_SOCKET

# Remove any leftover vsock sockets from previous runs
sudo rm -f /tmp/v.sock /tmp/*.vsock 2>/dev/null || true

LOGFILE="./firecracker.log"
sudo rm -f $LOGFILE

# Run firecracker (installed by vHive setup at /usr/local/bin/firecracker)
FIRECRACKER_BIN="/usr/local/bin/firecracker"
if [ ! -f "$FIRECRACKER_BIN" ]; then
    echo "ERROR: firecracker binary not found at $FIRECRACKER_BIN. Run setup_node first."
    exit 1
fi

sudo "$FIRECRACKER_BIN" --api-sock "${API_SOCKET}" --enable-pci