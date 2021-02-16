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
$PWD/setup_cri_dev_env.sh

if [ -f "/usr/local/go/bin/go" ]; then
    sudo rm -rf /usr/local/go
fi

# setup github runner
cd $HOME
if [ ! -d "$HOME/actions-runner" ]; then
    mkdir actions-runner && cd actions-runner
    LATEST_VERSION=$(curl -s https://api.github.com/repos/actions/runner/releases/latest | grep 'browser_' | cut -d\" -f4 | grep linux-x64)
    curl -o actions-runner-linux-x64.tar.gz -L -C - $LATEST_VERSION
    tar xzf "./actions-runner-linux-x64.tar.gz"
    rm actions-runner-linux-x64.tar.gz
    chmod +x ./config.sh
    chmod +x ./run.sh
    systemctl enable run-at-startup.service
    sudo tee /etc/systemd/system/connect_github_runner.service <<END
[Unit]
Description=Connect to Github as self hosted runner
Wants=network-online.target
After=network.target network-online.target

[Service]
Type=simple
RemainAfterExit=yes
Environment="RUNNER_ALLOW_RUNASROOT=1"
ExecStart=/root/actions-runner/run.sh
TimeoutStartSec=0

[Install]
WantedBy=default.target
END
else
    systemctl daemon-reload
    systemctl enable connect_github_runner --now

fi

# we want the command (expected to be systemd) to be PID1, so exec to it
exec "$@"
