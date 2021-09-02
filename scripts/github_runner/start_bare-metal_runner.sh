#!/bin/bash

# MIT License
#
# Copyright (c) 2020 Nathaniel Tornow and EASE lab
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

if [ -z $REPO_NAME ] || [ -z $RUNNER_TOKEN ] || [ -z $RUNNER_LABEL ] || [ -z $RUNNER_NAME ] || [ -z $SANDBOX ]; then
    echo "Parameters missing"
    exit -1
fi

URL="https://github.com/${REPO_NAME}"

sudo apt-get update

cd
git clone "https://github.com/${REPO_NAME}"
cd vhive
./scripts/cloudlab/setup_node.sh ${SANDBOX}

cd
mkdir actions-runner && cd actions-runner

curl -o actions-runner-linux-x64.tar.gz -L -C - $(curl -s https://api.github.com/repos/actions/runner/releases/latest | grep 'browser_' | cut -d\" -f4 | grep linux-x64)
tar xzf "./actions-runner-linux-x64.tar.gz"
rm actions-runner-linux-x64.tar.gz

./config.sh --url "https://github.com/${REPO_NAME}" --token ${RUNNER_TOKEN} --name ${RUNNER_NAME} --runnergroup default --labels ${RUNNER_LABEL} --work _work --replace

sudo ./svc.sh install root
sudo ./svc.sh start

