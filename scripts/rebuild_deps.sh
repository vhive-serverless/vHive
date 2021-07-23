# MIT License
#
# Copyright (c) 2020 Shyam Jesalpura and EASE lab
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

if [ -z $1 ] || [ -z $2 ]; then
    echo "Parameters missing"
    echo "USAGE: rebuild_deps.sh <fccd git branch> <Knative serving branch>"
    exit -1
fi
FCCD_BRANCH=$1
KNATIVE_BRANCH=$2

set -x
PWD="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
$PWD/install_go.sh
$PWD/install_docker.sh
sudo apt install -y git

cd $HOME
git clone -b $FCCD_BRANCH --recurse-submodules https://github.com/ease-lab/firecracker-containerd
cd firecracker-containerd
make image
make firecracker
make all

# copy binaries to vhive/bin
# cp runtime/containerd-shim-aws-firecracker $PWD/../bin/
# cp firecracker-control/cmd/containerd/firecracker-containerd $PWD/../bin/
# cp firecracker-control/cmd/containerd/firecracker-ctr $PWD/../bin/

# cp _submodules/firecracker/build/cargo_target/x86_64-unknown-linux-musl/release/firecracker $PWD/../bin/
# cp _submodules/firecracker/build/cargo_target/x86_64-unknown-linux-musl/release/jailer $PWD/../bin/

# install knative dependencies
VERSION=0.8.1 # choose the latest version
OS=Linux     # or Darwin
ARCH=x86_64  # or arm64, i386, s390x
curl -L https://github.com/google/ko/releases/download/v${VERSION}/ko_${VERSION}_${OS}_${ARCH}.tar.gz | tar xzf - ko
chmod +x ./ko
sudo mv ko /usr/bin/

wget -c https://github.com/google/protobuf/releases/download/v3.11.4/protoc-3.11.4-linux-x86_64.zip
sudo unzip protoc-3.11.4-linux-x86_64.zip -d /usr/local

export GOROOT=$(go env GOROOT)
go get github.com/gogo/protobuf/protoc-gen-gogofaster

#build knative
git clone -b $KNATIVE_BRANCH https://github.com/ease-lab/serving
$PWD/serving/hack/generate-yamls.sh $PWD/serving/ $PWD/serving/new-yamls.txt
