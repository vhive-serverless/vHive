#!/bin/bash

set -euo pipefail

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# Verify and activate the same receipt-backed runtime, demux and resolver.
# This never selects the historical binaries in vhive/bin.
exec python3 "$DIR/../firecracker_stack.py" activate "$@"
