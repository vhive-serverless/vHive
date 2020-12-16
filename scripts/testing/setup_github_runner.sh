# MIT License
#
# Copyright (c) 2020 Dmitrii Ustiugov, Plamen Petrov and EASE lab
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

#!/bin/bash

set -e

############## Supplementary functions below ##############
die () {
    echo >&2 "$@"
    exit 1
}

################### Main body below #######################

[ "$#" -eq 3 ] || die "1 argument required, $# provided"

RUNNER_NAME=$1
TOKEN=$2
EXTRA_LABELS=$3

sudo apt update
sudo apt-get install -y acl

sudo setfacl -m u:${USER}:rw /dev/kvm
sudo sh -c "echo always > /sys/kernel/mm/transparent_hugepage/defrag"
sudo sh -c "echo always > /sys/kernel/mm/transparent_hugepage/enabled"

cd /mnt/

./scripts/cloudlab/setup_node.sh

cd

# Create a folder
rm -rf actions-runner
mkdir actions-runner && cd actions-runner
curl -O -L https://github.com/actions/runner/releases/download/v2.274.2/actions-runner-linux-x64-2.274.2.tar.gz
tar xzf ./actions-runner-linux-x64-2.274.2.tar.gz

./config.sh --unattended \
    --url https://github.com/ease-lab/vhive \
    --token $TOKEN \
    --name $RUNNER_NAME \
    --labels Linux,X64,$EXTRA_LABELS

./run.sh
