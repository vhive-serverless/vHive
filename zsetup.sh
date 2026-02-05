#!/bin/bash
set -e  # Exit on error

# --- Configuration ---
SANDBOX=${1:-"firecracker"}
GO_VERSION="1.25.5"
BRANCH="zj-test1" 
export GITHUB_RUN_ID="local-test-$(date +%s)"
export GITHUB_VHIVE_ARGS="-dbg"
export REPO_URL="https://github.com/vhive-serverless/vHive.git"
export WORKSPACE_DIR="$HOME/vhive-local-test"

echo "--- 1. Environment Setup & Go $GO_VERSION Installation ---"
sudo apt update && sudo apt install -y rsync git git-lfs wget tar jq

# Install git-lfs
git lfs install

if ! go version 2>/dev/null | grep -q "go$GO_VERSION"; then
    wget -q https://go.dev/dl/go$GO_VERSION.linux-amd64.tar.gz
    sudo rm -rf /usr/local/go && sudo tar -C /usr/local -xzf go$GO_VERSION.linux-amd64.tar.gz
    rm go$GO_VERSION.linux-amd64.tar.gz
fi

sudo ln -sf /usr/local/go/bin/go /usr/bin/go
sudo ln -sf /usr/local/go/bin/gofmt /usr/bin/gofmt
export PATH=/usr/local/go/bin:$PATH

echo "--- 2. Checkout Code & Pull LFS Files ---"
if [ ! -d "$WORKSPACE_DIR" ]; then
    git clone -b "$BRANCH" "$REPO_URL" "$WORKSPACE_DIR"
else
    cd "$WORKSPACE_DIR"
    git fetch origin
    git checkout "$BRANCH"
    git pull origin "$BRANCH"
fi
cd "$WORKSPACE_DIR"
git lfs pull

echo "--- 3. Build Setup Tool ---"
cd scripts
go build -o setup_tool
cd ..

echo "--- 4. Setup Node ($SANDBOX) ---"
./scripts/setup_tool setup_node "$SANDBOX"

echo "--- 5. Configure PATH (persisting to ~/.bashrc) ---"
if ! grep -q "/usr/local/go/bin" ~/.bashrc; then
    echo 'export PATH=/usr/local/go/bin:$PATH' >> ~/.bashrc
fi

echo "--- 6. Setting executable permissions ---"
chmod +x "$WORKSPACE_DIR/zstart.sh" "$WORKSPACE_DIR/zboot.sh" 2>/dev/null || true

echo ""
echo "========================================="
echo "Setup Complete!"
echo "========================================="
echo "Next steps:"
echo "  1. Terminal 1: cd $WORKSPACE_DIR && ./zstart.sh"
echo "  2. Terminal 2: cd $WORKSPACE_DIR && ./zboot.sh"
echo "========================================="
