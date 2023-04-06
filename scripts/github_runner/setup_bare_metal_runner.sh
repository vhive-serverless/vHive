#!/bin/bash

# MIT License
#
# Copyright (c) 2020 Nathaniel Tornow and EASE lab
#
# Permission is hereby granted, free of charge, to any person obtaining a copy
# of this software and associated documentation files (the "Software"), to deal
# in the Software without restriction, including without limitation the rights
# to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
# copies of the Software, and to permit persons to whom the Software is
# furnished to do so, subject to the following conditions:
#
# The above copyright notice and this permission notice shall be included in all
# copies or substantial portions of the Software.
#
# THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
# IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
# FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
# AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
# LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
# OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
# SOFTWARE.

set -Eeo pipefail

cd "$( dirname "${BASH_SOURCE[0]}" )"

if (( $# < 3)); then
    echo "Invalid number of parameters"
    echo "USAGE: setup_bare_metal_runner.sh <GitHub organization> <Github PAT> <sandbox> [restart]"
    exit 1
fi

GH_ORG=$1
GH_PAT=$2
SANDBOX=$3
RESTART=$4

VHIVE_ROOT="$(git rev-parse --show-toplevel)"
"$VHIVE_ROOT"/scripts/cloudlab/setup_node.sh "$SANDBOX"

cd
export RUNNER_ALLOW_RUNASROOT=1
export RUNNER_CFG_PAT=$GH_PAT
RUNNER_NAME=$RUNNER_LABEL-${HOSTNAME//[.]/-}
RUNNER_NAME=${RUNNER_NAME:0:64}
RUNNER_LABEL=$SANDBOX-cri
curl -s https://raw.githubusercontent.com/actions/runner/main/scripts/create-latest-svc.sh | bash -s - -s "$GH_ORG"/vhive -n "$RUNNER_NAME" -l "$RUNNER_LABEL" -f

echo "0 4 * * * root reboot" | sudo tee -a /etc/crontab

