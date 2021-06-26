#!/bin/bash

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

DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null 2>&1 && pwd )"
ROOT="$( cd $DIR && cd .. && cd .. && pwd)"
SCRIPTS=$ROOT/scripts

GVISOR=$1

echo Clean up host resources if left after previous runs
$SCRIPTS/github_runner/clean_cri_runner.sh

CTRDLOGDIR=/tmp/ctrd-logs/$GITHUB_RUN_ID
sudo mkdir -p -m777 -p $CTRDLOGDIR

echo Run the stock containerd daemon
sudo containerd 1>$CTRDLOGDIR/ctrd.out 2>$CTRDLOGDIR/ctrd.err &
sleep 1s

if [ "$GVISOR" == "gvisor" ]; then
    echo Run the gvisor-containerd daemon
    sudo /usr/local/bin/gvisor-containerd --address /run/gvisor-containerd/gvisor-containerd.sock --config /etc/gvisor-containerd/config.toml 1>$CTRDLOGDIR/gvisor.out 2>$CTRDLOGDIR/gvisor.err &
else
    echo Run the firecracker-containerd daemon
    sudo /usr/local/bin/firecracker-containerd --config /etc/firecracker-containerd/config.toml 1>$CTRDLOGDIR/fccd.out 2>$CTRDLOGDIR/fccd.err &
fi

sleep 1s

echo Build vHive
cd $ROOT
source /etc/profile && go build

echo Running vHive with \"${GITHUB_VHIVE_ARGS}\" arguments
sudo ./vhive ${GITHUB_VHIVE_ARGS} 1>$CTRDLOGDIR/orch.out 2>$CTRDLOGDIR/orch.err &
sleep 1s

$SCRIPTS/cluster/create_one_node_cluster.sh

echo All logs are stored in $CTRDLOGDIR
