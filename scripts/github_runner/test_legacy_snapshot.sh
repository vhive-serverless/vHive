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
        if ! bash "$runtime_source/tools/thinpool.sh" remove "$FICD_DM_POOL" \
            > "$VHIVE_CI_EVIDENCE/pool-cleanup.log" 2>&1; then
            result=1
        fi
    fi
    exit "$result"
}
trap cleanup EXIT
bash "$runtime_source/tools/thinpool.sh" create "$FICD_DM_POOL" \
    2>&1 | tee "$VHIVE_CI_EVIDENCE/pool-create.log"
pool_created=true

# The existing runner verifies published module origins, the installed receipt,
# actual rendered configuration, and all three required File-memory VM tests.
# UPF/lazy, native/CRI restore and Kubernetes endpoints are separate scopes.
sudo -n env "PATH=$PATH" python3 scripts/accept_legacy_snapshot.py \
    --prefix "$INSTALL_DIR" --runtime-revision "$revision" \
    --go "$(command -v go)" --output "$VHIVE_CI_EVIDENCE/acceptance" \
    2>&1 | tee "$VHIVE_CI_EVIDENCE/acceptance-runner.log"
