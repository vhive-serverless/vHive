# MIT License
#
# Copyright (c) 2020 Dmitrii Ustiugov, Shyam Jesalpura and EASE lab
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

PWD="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
$PWD/setup_system.sh
$PWD/create_devmapper.sh

# install Protocol Buffer Compiler
PROTO_VERSION=3.11.4
if [ ! -f "protoc-$PROTO_VERSION-linux-x86_64.zip" ]; then
    wget -c "https://github.com/google/protobuf/releases/download/v$PROTO_VERSION/protoc-$PROTO_VERSION-linux-x86_64.zip"
    sudo unzip -u "protoc-$PROTO_VERSION-linux-x86_64.zip" -d /usr/local
    rm "protoc-$PROTO_VERSION-linux-x86_64.zip"
fi

# Necessary for containerd as container runtime but not docker
sudo modprobe overlay
sudo modprobe br_netfilter

# Set up required sysctl params, these persist across reboots.
sudo tee /etc/sysctl.d/99-kubernetes-cri.conf <<EOF
net.bridge.bridge-nf-call-iptables  = 1
net.ipv4.ip_forward                 = 1
net.bridge.bridge-nf-call-ip6tables = 1
EOF

sudo sysctl --system
sudo swapoff -a

# we want the command (expected to be systemd) to be PID1, so exec to it
exec "$@"
