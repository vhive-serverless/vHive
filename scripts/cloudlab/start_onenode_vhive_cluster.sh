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

$SCRIPTS/github_runner/clean_cri_runner.sh

CTRDLOGDIR=/tmp/ctrd-logs/$GITHUB_RUN_ID

sudo mkdir -p -m777 -p $CTRDLOGDIR

sudo containerd 1>$CTRDLOGDIR/ctrd.out 2>$CTRDLOGDIR/ctrd.err &
sleep 1s
sudo /usr/local/bin/firecracker-containerd --config /etc/firecracker-containerd/config.toml 1>$CTRDLOGDIR/fccd.out 2>$CTRDLOGDIR/fccd.err &
sleep 1s

if [ ! -f "$ROOT/vhive" ]; then
    source /etc/profile && cd $ROOT && go build
fi
sleep 1s

cd $ROOT && sudo ./vhive -dbg 1>$CTRDLOGDIR/orch.out 2>$CTRDLOGDIR/orch.err &
sleep 1s

$SCRIPTS/cluster/create_one_node_cluster.sh
