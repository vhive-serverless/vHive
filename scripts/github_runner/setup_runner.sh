#!/bin/bash
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

# When executed inside a docker container, this command returns the container ID of the container.
# on a non container environment, this returns "/".
CONTAINERID=$(basename $(cat /proc/1/cpuset))

# Docker container ID is 64 characters long.
if [ 64 -eq ${#CONTAINERID} ]; then
  # set thinpool device name dynamically
  sudo sed -i "s/fc-dev-thinpool/${CONTAINERID}_thinpool/" /etc/firecracker-containerd/config.toml
fi

/create_devmapper.sh

sudo apt-get install linux-tools-`uname -r` -y

if [ ! -d "/usr/local/pmu-tools" ]; then
  ln -s /usr/bin/python3 /usr/bin/python
  /install_pmutools.sh
fi