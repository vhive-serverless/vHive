#!/bin/bash

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

PWD="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"

if (( $# < 3)); then
    echo "Parameters missing"
    echo "USAGE: start_runners.sh <number of runners> <GitHub organization> <Github PAT> [restart]"
    exit 1
fi

NUM_OF_RUNNERS=$1
GH_ORG=$2
GH_PAT=$3
RESTART=$4

if [ "$RESTART" != "restart" ]; then
  echo "0 4 * * * root reboot" | sudo tee -a /etc/crontab
fi

# Based on https://github.com/actions/runner/blob/0484afeec71b612022e35ba80e5fe98a99cd0be8/scripts/create-latest-svc.sh#L112-L131
RUNNER_TOKEN=$(curl -s -X POST https://api.github.com/repos/"$GH_ORG"/vhive/actions/runners/registration-token -H "accept: application/vnd.github.everest-preview+json" -H "authorization: token $GH_PAT" | jq -r '.token')
if [ "null" == "$RUNNER_TOKEN" ] || [ -z "$RUNNER_TOKEN" ]; then
  echo "Failed to get a runner token"
  exit 1
fi

# pull latest images
docker pull vhiveease/integ_test_runner:ubuntu20base

if [ "$RESTART" == "restart" ]; then
    docker container stop $(docker ps --format "{{.Names}}" | grep integration_test-github_runner | xargs)
    docker container rm $(docker ps -a --format "{{.Names}}" | grep integration_test-github_runner | xargs)
fi

HOSTNAME=${HOSTNAME//[.]/-}
HOSTNAME=${HOSTNAME:0:32}
for num in $(seq 1 "$NUM_OF_RUNNERS")
do
    docker run -d --restart always --privileged \
        --name "integration_test-github_runner-$num" \
        -e _SHORT_URL="https://github.com/$GH_ORG/vHive" \
        -e RUNNER_TOKEN="$RUNNER_TOKEN" \
        -e LABELS="integ" \
        -e HOSTNAME="$HOSTNAME" \
        -e NUMBER="$num" \
        --ipc=host \
        -v /var/run/docker.sock:/var/run/docker.sock \
        --volume /dev:/dev \
        --volume /run/udev/control:/run/udev/control \
        vhiveease/integ_test_runner:ubuntu20base
done
