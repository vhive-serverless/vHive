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

set -e

VHIVE_ROOT="$(git rev-parse --show-toplevel)"

arch=`uname -m`

case $arch in
  'x86_64')
    arch='amd64'
    ;;
  *)
    echo "Unsupported architecture $arch"
    exit 1
    ;;
esac

sudo apt update; sudo apt install jq -y

go_version=`cat $VHIVE_ROOT/configs/setup/system.json | jq '.GoVersion' | sed 's/"//g'`
go_link=`cat $VHIVE_ROOT/configs/setup/system.json | jq '.GoDownloadUrlTemplate' | sed 's/"//g'`
go_link=`echo $go_link | sed 's/%s/'$go_version'/' | sed 's/%s/'$arch'/'`

echo Installing go version $go_version

wget --continue --quiet $go_link

sudo tar -C /usr/local -xzf ${go_link##*/}

export PATH=$PATH:/usr/local/go/bin

sudo sh -c  "echo 'export PATH=\$PATH:/usr/local/go/bin' >> /etc/profile"
