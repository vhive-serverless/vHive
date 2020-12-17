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

[ "$#" -eq 3 ] || die "3 argument required, $# provided"

RUNNER_NAME=$1
TOKEN=$2
EXTRA_LABELS=$3

sudo apt update
sudo apt-get install -y acl

sudo setfacl -m u:${USER}:rw /dev/kvm
sudo sh -c "echo always > /sys/kernel/mm/transparent_hugepage/defrag"
sudo sh -c "echo always > /sys/kernel/mm/transparent_hugepage/enabled"

cd /mnt/

if [[ $EXTRA_LABELS =~ (ctrd) ]]; then
    echo Setting up a runner for single-host experiments

    sudo apt-get install -y docker.io
    sudo usermod -aG docker $USER

    ./scripts/travis/setup_node.sh
else
    echo Setting up a runner for cluster experiments
    ./scripts/cloudlab/setup_node.sh
fi

cd

echo Run local registry in a Docker container
sudo docker run -d -p 5000:5000 --restart=always --name registry registry:2

declare -a test_func
test_func[0]="helloworld"
test_func[1]="chameleon"
test_func[2]="pyaes"
test_func[3]="image_rotate"
test_func[4]="json_serdes"
test_func[5]="lr_serving"
test_func[6]="cnn_serving"
test_func[7]="rnn_serving"
test_func[8]="lr_training"

for f in ${test_func[@]}; do
    sudo docker pull vhiveease/$f:var_workload
    sudo docker tag vhiveease/$f:var_workload localhost:5000/vhiveease/$f:var_workload
    sudo docker push localhost:5000/vhiveease/$f:var_workload
done

# Create a folder
rm -rf actions-runner
mkdir actions-runner && cd actions-runner

latest_version=$(curl -s "https://github.com/actions/runner/releases/latest/download" 2>&1 | sed "s/^.*download\/v\([^\"]*\).*/\1/")
curl -O -L https://github.com/actions/runner/releases/download/v${latest_version}/actions-runner-linux-x64-${latest_version}.tar.gz
tar xzf ./actions-runner-linux-x64-${latest_version}.tar.gz

./config.sh --unattended \
    --url https://github.com/ease-lab/vhive \
    --token $TOKEN \
    --name $RUNNER_NAME \
    --labels Linux,X64,$EXTRA_LABELS
