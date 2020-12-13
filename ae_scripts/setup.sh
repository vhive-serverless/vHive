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

# Kill all children of the process upon a keyboard interrupt or exit
trap "exit" INT TERM ERR
trap "kill 0" EXIT

DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null 2>&1 && pwd )"

ROOT="$( cd $DIR && cd .. && pwd)"
SCRIPTS=$ROOT/scripts

# Set up KVM
sudo setfacl -m u:${USER}:rw /dev/kvm

# Check if KVM is available
[ -r /dev/kvm ] && [ -w /dev/kvm ] && echo "KVM is available" || echo "KVM is unavailable"

source $SCRIPTS/install_go.sh
$SCRIPTS/setup_system.sh

sudo apt-get -y install gcc g++ acl gcc g++ make acl net-tools

$SCRIPTS/setup_containerd.sh

$SCRIPTS/create_devmapper.sh

echo Set up MinIO server
$ROOT/function-images/minio_scripts/install_minio.sh

echo Run MinIO server as a daemon
$ROOT/function-images/minio_scripts/start_minio_server.sh &
sleep 1

host_ip=`curl ifconfig.me`
$ROOT/function-images/minio_scripts/create_minio_bucket.sh http://$host_ip:9000 || echo Minio bucket exists, continuing...

echo Populate the bucket with all files
$ROOT/function-images/minio_scripts/put_in_bucket.sh

echo Contents of the MinIO bucket:
mc ls myminio/mybucket

sudo pkill -9 minio || echo

echo Build the project

source /etc/profile
GO_BIN=`which go`

go build
