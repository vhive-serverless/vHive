#!/bin/bash

# MIT License
#
# Copyright (c) 2023 Dmitrii Ustiugov, Anshal Shukla, Georgiy Lebedev and EASE lab
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

PWD="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null 2>&1 && pwd )"

RUNNER_LABEL=$1

echo "$RUNNER_LABEL"

# setup cluster
./scripts/cloudlab/setup_node.sh "$RUNNER_LABEL"
./scripts/cloudlab/start_onenode_vhive_cluster.sh "$RUNNER_LABEL"
sleep 2m

# deploy functions
kn service apply helloworld -f ./configs/knative_workloads/helloworld.yaml
kn service apply helloworldserial -f ./configs/knative_workloads/helloworldSerial.yaml
kn service apply pyaes -f ./configs/knative_workloads/pyaes.yaml
sleep 1m

cd cri

go clean -testcache
source /etc/profile && go test . -v -race -cover
