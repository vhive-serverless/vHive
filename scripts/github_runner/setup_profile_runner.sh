#!/bin/bash

# MIT License
#
# Copyright (c) 2023 Lai Ruiqi, Dmitrii Ustiugov and vHive team
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

# setup runner for profile unit test
GH_ORG=$1
GH_PAT=$2

sudo apt-get update
sudo apt-get install -y jq tmux

# Based on https://github.com/actions/runner/blob/0484afeec71b612022e35ba80e5fe98a99cd0be8/scripts/create-latest-svc.sh#L112-L131
RUNNER_TOKEN=$(curl -s -X POST https://api.github.com/repos/"$GH_ORG"/vhive/actions/runners/registration-token -H "accept: application/vnd.github.everest-preview+json" -H "authorization: token $GH_PAT" | jq -r '.token')
if [ "null" == "$RUNNER_TOKEN" ] || [ -z "$RUNNER_TOKEN" ]; then
  echo "Failed to get a runner token"
  exit 1
fi

cd $HOME
if [ ! -d "$HOME/actions-runner" ]; then
    mkdir actions-runner && cd actions-runner
    LATEST_VERSION=$(curl -s https://api.github.com/repos/actions/runner/releases/latest | grep 'browser_' | cut -d\" -f4 | grep 'linux-x64-[0-9\.]*.tar.gz')
    curl -o actions-runner-linux-x64.tar.gz -L -C - $LATEST_VERSION
    tar xzf "./actions-runner-linux-x64.tar.gz"
    rm actions-runner-linux-x64.tar.gz
    chmod +x ./config.sh
    chmod +x ./run.sh
    RUNNER_ALLOW_RUNASROOT=1 ./config.sh --url "https://github.com/$GH_ORG/vHive" \
                    --token "${RUNNER_TOKEN}" \
                    --name "profile-test-github-runner" \
                    --work "$HOME/actions-runner/_work" \
                    --labels "profile" \
                    --unattended \
                    --replace

fi

cd $HOME/actions-runner
tmux new-session -d -s session_name 'RUNNER_ALLOW_RUNASROOT=1 ./run.sh'
echo "SETUP PROFILE FINISHED"