#!/usr/bin/env bash
# Run only on the disposable KVM runner owned by this CI job.
set -euo pipefail

repo_dir=$(cd "$(dirname "$0")/../.." && pwd)
cd "$repo_dir"
: "${STACK_DIR:?Set the private CI build directory}"
: "${INSTALL_DIR:?Set the private CI installation prefix}"
: "${VHIVE_CI_EVIDENCE:?Set the evidence directory outside the checkout}"
: "${FICD_DM_POOL:?Set the unique CI-owned thin-pool name}"
export FICD_DM_VOLUME_GROUP=''
export GOWORK=off
# Final CI must consume the committed pin, never a development override.
unset VHIVE_RUNTIME_REVISION VHIVE_RUNTIME_CHECKOUT

mkdir -p "$VHIVE_CI_EVIDENCE"
revision=$(python3 - <<'PY'
import json
from pathlib import Path
manifest = json.loads(Path("configs/setup/firecracker-stack.json").read_text())
if not manifest["runtime"]["ready"]:
    raise SystemExit("Finalize the accepted Step 2 runtime/module pin before legacy snapshot CI")
print(manifest["runtime"]["revision"])
PY
)
python3 scripts/firecracker_stack.py install --prepare --activate --pool "$FICD_DM_POOL" \
    2>&1 | tee "$VHIVE_CI_EVIDENCE/install.log"
cp "$INSTALL_DIR/vhive-stack-receipt.json" "$VHIVE_CI_EVIDENCE/"

runtime_source="$STACK_DIR/sources/$revision"
pool_created=false
cleanup() {
    result=$?
    trap - EXIT
    if [[ "$pool_created" == true ]]; then
        if [[ -f "$VHIVE_CI_EVIDENCE/pool/ownership.json" ]] && \
            ! python3 scripts/legacy_snapshot_pool.py remove \
                --receipt "$VHIVE_CI_EVIDENCE/pool/ownership.json" \
                > "$VHIVE_CI_EVIDENCE/pool-cleanup.log" 2>&1; then
            result=1
        fi
    fi
    exit "$result"
}
trap cleanup EXIT
# The runtime's generic helper defaults to 10G; this 10GB disk profile needs
# room for the source and two full independent clone copies plus the base image.
pool_created=true
python3 scripts/legacy_snapshot_pool.py create --runtime-source "$runtime_source" \
    --pool "$FICD_DM_POOL" --evidence "$VHIVE_CI_EVIDENCE/pool" \
    2>&1 | tee "$VHIVE_CI_EVIDENCE/pool-create.log"

# The existing runner verifies published module origins, the installed receipt,
# actual rendered configuration, and all three required File-memory VM tests.
# UPF/lazy, native/CRI restore and Kubernetes endpoints are separate scopes.
sudo -n env "PATH=$PATH" python3 scripts/accept_legacy_snapshot.py \
    --prefix "$INSTALL_DIR" --runtime-revision "$revision" \
    --go "$(command -v go)" --output "$VHIVE_CI_EVIDENCE/acceptance" \
    2>&1 | tee "$VHIVE_CI_EVIDENCE/acceptance-runner.log"
