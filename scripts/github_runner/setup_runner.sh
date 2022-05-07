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

# setup github runner
cd $HOME
if [ ! -d "$HOME/actions-runner" ]; then
    mkdir actions-runner && cd actions-runner
    LATEST_VERSION=$(curl -s https://api.github.com/repos/actions/runner/releases/latest | grep 'browser_' | cut -d\" -f4 | grep 'linux-x64-[0-9\.]*.tar.gz')
    curl -o actions-runner-linux-x64.tar.gz -L -C - $LATEST_VERSION
    tar xzf "./actions-runner-linux-x64.tar.gz"
    rm actions-runner-linux-x64.tar.gz
    chmod +x ./config.sh
    chmod +x ./run.sh
    RUNNER_ALLOW_RUNASROOT=1 ./config.sh --url "${_SHORT_URL}" \
                    --token "${RUNNER_TOKEN}" \
                    --name "integ-test-github-runner-${HOSTNAME}-${NUMBER}" \
                    --work "/root/_work" \
                    --labels "integ" \
                    --unattended \
                    --replace

fi

cd $HOME/actions-runner
RUNNER_ALLOW_RUNASROOT=1 ./run.sh

