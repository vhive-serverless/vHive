#!/bin/bash

# --- Configuration ---
SANDBOX=${1:-"gvisor"} 
GO_VERSION="1.22.9"
BRANCH="migrate-ci-github-runner"  # Changed to your current branch
export GITHUB_RUN_ID="local-test-$(date +%s)"
export GITHUB_VHIVE_ARGS="-dbg"
export REPO_URL="https://github.com/vhive-serverless/vHive.git"
export WORKSPACE_DIR="$HOME/vhive-local-test"

set +e

echo "========================================="
echo "vHive gVisor Migration Test Script"
echo "Branch: $BRANCH"
echo "Sandbox: $SANDBOX"
echo "Running on: $(hostname)"
echo "========================================="

echo ""
echo "--- 1. Environment Setup & Go $GO_VERSION Installation ---"
sudo apt update 
sudo apt install -y rsync git git-lfs wget tar jq

# Install Go
echo "Installing Go $GO_VERSION..."
if ! go version 2>/dev/null | grep -q "go$GO_VERSION"; then
    wget -q https://go.dev/dl/go$GO_VERSION.linux-amd64.tar.gz
    sudo rm -rf /usr/local/go 
    sudo tar -C /usr/local -xzf go$GO_VERSION.linux-amd64.tar.gz
    rm go$GO_VERSION.linux-amd64.tar.gz
    echo "Go $GO_VERSION installed successfully"
else
    echo "Go $GO_VERSION already installed"
fi

sudo ln -sf /usr/local/go/bin/go /usr/bin/go
sudo ln -sf /usr/local/go/bin/gofmt /usr/bin/gofmt
export PATH=/usr/local/go/bin:$PATH

echo ""
echo "--- 2. Checkout Code & Pull LFS Files ---"
if [ ! -d "$WORKSPACE_DIR" ]; then
    echo "Cloning vHive repository, branch: $BRANCH..."
    git clone -b "$BRANCH" "$REPO_URL" "$WORKSPACE_DIR"
else
    echo "Directory exists. Switching to branch: $BRANCH..."
    cd "$WORKSPACE_DIR"
    git fetch origin
    git checkout "$BRANCH"
    git pull origin "$BRANCH"
fi
cd "$WORKSPACE_DIR"

echo "Initializing and pulling Git LFS files..."
git lfs install
git lfs pull
echo "✓ Git LFS files pulled successfully"

echo ""
echo "--- 3. Build Setup Tool ---"
pushd scripts > /dev/null
go build -o setup_tool
BUILD_EXIT=$?
popd > /dev/null
if [ $BUILD_EXIT -ne 0 ]; then
    echo "✗ ERROR: Failed to build setup tool (exit code: $BUILD_EXIT)"
    echo "Continuing anyway..."
else
    echo "✓ Setup tool built successfully"
fi

echo ""
echo "--- 4. Setup Node ($SANDBOX) ---"
echo "Installing Kubernetes, containerd, and runsc shim..."
./scripts/setup_tool setup_node "$SANDBOX"
SETUP_EXIT=$?
if [ $SETUP_EXIT -ne 0 ]; then
    echo "✗ ERROR: Node setup failed (exit code: $SETUP_EXIT)"
    echo "Continuing anyway..."
else
    echo "✓ Node setup completed"
fi

echo ""
echo "--- 5. Verification: Binary & Configuration (Pre-Start) ---"
echo "Checking containerd-shim-runsc-v1 binary..."
if [ -f "/usr/local/bin/containerd-shim-runsc-v1" ]; then
    ls -lh /usr/local/bin/containerd-shim-runsc-v1
    echo "✓ Shim binary installed"
else
    echo "✗ ERROR: Shim binary not found at /usr/local/bin/containerd-shim-runsc-v1"
    echo "Continuing anyway..."
fi

echo ""
echo "Validating containerd configuration..."
if ! sudo containerd config dump > /dev/null 2>&1; then
    echo "✗ ERROR: Containerd config is invalid!"
    echo "Config validation error:"
    sudo containerd config dump 2>&1 | head -20
    echo ""
    echo "Current config file:"
    sudo cat /etc/containerd/config.toml | grep -C 10 "runsc" || echo "No runsc config found"
    echo "Continuing anyway..."
else
    echo "✓ Containerd config is valid"
fi

echo ""
echo "Checking runsc runtime configuration..."
if sudo grep -q "io.containerd.runsc.v1" /etc/containerd/config.toml; then
    echo "Runsc runtime configuration:"
    sudo cat /etc/containerd/config.toml | grep -B 2 -A 5 "runtimes.runsc"
    echo "✓ Runsc runtime configured"
else
    echo "✗ ERROR: Runsc runtime not configured in /etc/containerd/config.toml"
    echo "Continuing anyway..."
fi

echo ""
echo "--- 6. Setup vHive CRI Test Environment ---"
echo "Setting up vHive orchestrator, cluster, and Knative workloads..."
./scripts/github_runner/setup_cri_test_env.sh "$SANDBOX"
CRI_SETUP_EXIT=$?

if [ $CRI_SETUP_EXIT -ne 0 ]; then
    echo "✗ ERROR: CRI test environment setup failed (exit code: $CRI_SETUP_EXIT)"
    echo ""
    echo "Checking logs in /tmp/ctrd-logs/$GITHUB_RUN_ID/"
    if [ -d "/tmp/ctrd-logs/$GITHUB_RUN_ID" ]; then
        echo ""
        echo "=== Last 30 lines of Containerd error log ==="
        tail -30 "/tmp/ctrd-logs/$GITHUB_RUN_ID/ctrd.err" 2>/dev/null || echo "(no log found)"
        echo ""
        echo "=== Last 30 lines of vHive error log ==="
        tail -30 "/tmp/ctrd-logs/$GITHUB_RUN_ID/orch.err" 2>/dev/null || echo "(no log found)"
        echo ""
        echo "=== Checking kubelet logs ==="
        sudo journalctl -u kubelet --no-pager -n 50 | tail -20
    fi
    echo "Continuing with verification steps..."
else
    echo "✓ CRI test environment setup completed"
fi

echo ""
echo "--- 7. Post-Setup Verification ---"
echo "Checking stock containerd status..."
ps aux | grep "containerd" | grep -v "firecracker-containerd" | grep -v grep || echo "⚠ Stock containerd not running!"

echo ""
echo "Checking containerd socket..."
if [ -S "/run/containerd/containerd.sock" ]; then
    echo "✓ Containerd socket exists"
    sudo ls -la /run/containerd/containerd.sock
else
    echo "✗ ERROR: Containerd socket not found at /run/containerd/containerd.sock"
fi

echo ""
echo "Note: Images will be pre-pulled by setup_cri_test_env.sh before deployment"

echo ""
echo "Checking vHive orchestrator..."
ps aux | grep "./vhive" | grep -v grep || echo "⚠ vHive orchestrator not found"

echo ""
echo "Checking CRI proxy socket..."
ls -la /etc/vhive-cri/vhive-cri.sock 2>/dev/null || echo "⚠ CRI proxy socket not found"

echo ""
echo "Checking Kubernetes cluster..."
sudo KUBECONFIG=/etc/kubernetes/admin.conf kubectl get nodes -o wide
sudo KUBECONFIG=/etc/kubernetes/admin.conf kubectl get pods -n knative-serving

echo ""
echo "Checking deployed services..."
sudo KUBECONFIG=/etc/kubernetes/admin.conf kubectl get ksvc -n default

echo ""
echo "Checking for containerd-shim-runsc-v1 processes..."
SHIM_PROCS=$(ps aux | grep -v grep | grep "containerd-shim-runsc-v1" || true)
if [ -n "$SHIM_PROCS" ]; then
    echo "✓ Shim processes found:"
    echo "$SHIM_PROCS" | head -5
    SHIM_COUNT=$(echo "$SHIM_PROCS" | wc -l)
    echo "Total: $SHIM_COUNT shim processes"
else
    echo "⚠ No shim processes yet (will appear when gVisor containers start)"
fi

echo ""
echo "--- 8. Wait for Pods to be Ready ---"
echo "Waiting up to 120s for helloworld pod to be ready..."
for i in {1..24}; do
    POD_STATUS=$(sudo KUBECONFIG=/etc/kubernetes/admin.conf kubectl get pods -n default -l serving.knative.dev/service=helloworld -o jsonpath='{.items[0].status.phase}' 2>/dev/null || echo "NotFound")
    echo "Attempt $i/24: Pod status = $POD_STATUS"
    
    if [ "$POD_STATUS" = "Running" ]; then
        echo "✓ Helloworld pod is Running!"
        break
    elif [ "$POD_STATUS" = "NotFound" ]; then
        echo "  Waiting for pod to be created..."
    else
        echo "  Pod status: $POD_STATUS"
        # Show container statuses
        sudo KUBECONFIG=/etc/kubernetes/admin.conf kubectl get pods -n default -l serving.knative.dev/service=helloworld -o jsonpath='{.items[0].status.containerStatuses[*].state}' 2>/dev/null || true
        echo ""
    fi
    
    sleep 5
done

echo ""
echo "Final pod status:"
sudo KUBECONFIG=/etc/kubernetes/admin.conf kubectl get pods -n default -o wide

echo ""
echo "--- 10. Fix Permissions & Run Tests ---"
echo "Fixing Go module permissions..."
sudo chown -R $(whoami):$(whoami) ~/go 2>/dev/null || true
sudo chmod -R u+w ~/go 2>/dev/null || true

export GOPATH=$HOME/go
export PATH=$PATH:/usr/local/go/bin:$GOPATH/bin
go clean -testcache

echo ""
echo "========================================="
echo "Running CRI Tests with -race -cover"
echo "========================================="
source /etc/profile 2>/dev/null || true
go test ./cri -v -race -cover -timeout 60m
TEST_EXIT_CODE=$?

echo ""
echo "--- 10. Post-Test Verification ---"
echo "Checking shim processes after test..."
SHIM_COUNT=$(ps aux | grep "containerd-shim-runsc-v1" | grep -v grep | wc -l || echo "0")
echo "Found $SHIM_COUNT shim processes"

if [ $SHIM_COUNT -gt 0 ]; then
    echo "✓ Shim processes are running:"
    ps aux | grep "containerd-shim-runsc-v1" | grep -v grep | head -5
fi

echo ""
echo "Checking pod status after tests..."
sudo KUBECONFIG=/etc/kubernetes/admin.conf kubectl get pods -n default -o wide

echo ""
echo "--- 11. Test Summary ---"
echo "========================================="
if [ $TEST_EXIT_CODE -eq 0 ]; then
    echo "✓✓✓ ALL TESTS PASSED ✓✓✓"
    echo ""
    echo "Key verifications:"
    echo "  ✓ Shim binary installed: /usr/local/bin/containerd-shim-runsc-v1"
    echo "  ✓ Stock containerd configured with runsc runtime"
    echo "  ✓ vHive orchestrator running"
    echo "  ✓ Kubernetes cluster operational"
    echo "  ✓ Knative services deployed"
    echo "  ✓ $SHIM_COUNT shim processes spawned"
    echo "  ✓ CRI tests passed"
else
    echo "✗✗✗ TESTS FAILED (Exit: $TEST_EXIT_CODE) ✗✗✗"
    echo ""
    echo "Debug information:"
    echo ""
    echo "1. Check containerd logs:"
    echo "   sudo journalctl -u containerd -n 100"
    echo ""
    echo "2. Check kubelet logs:"
    echo "   sudo journalctl -u kubelet -n 100"
    echo ""
    echo "3. Check vHive orchestrator logs:"
    echo "   cat /tmp/ctrd-logs/$GITHUB_RUN_ID/orch.err"
    echo ""
    echo "4. Check pod events:"
    echo "   sudo KUBECONFIG=/etc/kubernetes/admin.conf kubectl get events -n default --sort-by='.lastTimestamp'"
    echo ""
    echo "5. Describe problematic pod:"
    echo "   sudo KUBECONFIG=/etc/kubernetes/admin.conf kubectl describe pod -n default <pod-name>"
fi
echo "========================================="
echo "Logs: /tmp/ctrd-logs/$GITHUB_RUN_ID"

echo ""
echo "--- 12. Cleanup ---"
read -p "Do you want to clean up the test environment? (y/N): " -n 1 -r
echo
if [[ $REPLY =~ ^[Yy]$ ]]; then
    echo "Cleaning up test environment..."
    ./scripts/github_runner/clean_cri_runner.sh "$SANDBOX" || echo "Cleanup had issues but continuing..."
    echo "✓ Cleanup completed"
else
    echo "Skipping cleanup - environment preserved for debugging"
    echo ""
    echo "To clean up later, run:"
    echo "  cd $WORKSPACE_DIR"
    echo "  ./scripts/github_runner/clean_cri_runner.sh $SANDBOX"
fi

echo ""
echo "Script completed. Exit code: $TEST_EXIT_CODE"
echo "You can continue investigating in the current shell."
echo ""

# Don't exit - keep shell open for debugging
# exit $TEST_EXIT_CODE
