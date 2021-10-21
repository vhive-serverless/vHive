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

if [ -z $1 ] || [ -z $2 ] || [ -z $3 ] || [ -z $4 ]; then
    echo "Parameters missing"
    echo "USAGE: start_runners.sh <num of runners> https://github.com/<OWNER>/<REPO> <Github Access key> <runner label(comma separated)> [restart]" 
    exit -1
fi

NUM_OF_RUNNERS=$1
RUNNER_LABEL=$4
RESTART_FLAG=$5

# fetch runner token using access token
ACCESS_TOKEN=$3
API_VERSION=v3
API_HEADER="Accept: application/vnd.github.${API_VERSION}+json"
AUTH_HEADER="Authorization: token ${ACCESS_TOKEN}"

_SHORT_URL=$2
REPO_NAME="$(echo "${_SHORT_URL}" | grep / | cut -d/ -f4-)"
_FULL_URL="https://api.github.com/repos/${REPO_NAME}/actions/runners/registration-token"

RUNNER_TOKEN="$(curl -XPOST -fsSL \
  -H "${AUTH_HEADER}" \
  -H "${API_HEADER}" \
  "${_FULL_URL}" \
| jq -r '.token')"

case "$RUNNER_LABEL" in
"integ")
    # pull latest images
    docker pull vhiveease/integ_test_runner:ubuntu20base
    docker pull vhiveease/cri_test_runner 

    if [ "$RESTART_FLAG" == "restart" ]; then
        docker container stop $(docker ps --format "{{.Names}}" | grep integration_test-github_runner)
        docker container rm $(docker ps -a --format "{{.Names}}" | grep integration_test-github_runner)
    fi
    for number in $(seq 1 $NUM_OF_RUNNERS)
    do
        # create access token as mentioned here (https://github.com/myoung34/docker-github-actions-runner#create-github-personal-access-token)
        docker run -d --restart always --privileged \
            --name "integration_test-github_runner-${HOSTNAME}-${number}" \
            -e _SHORT_URL="${_SHORT_URL}" \
            -e RUNNER_TOKEN="${RUNNER_TOKEN}" \
            -e LABELS="${RUNNER_LABEL}" \
            -e HOSTNAME="${HOSTNAME}" \
            -e NUMBER="${number}" \
            --ipc=host \
            -v /var/run/docker.sock:/var/run/docker.sock \
            --volume /dev:/dev \
            --volume /run/udev/control:/run/udev/control \
            vhiveease/integ_test_runner:ubuntu20base
    done
    ;;
"cri")
    # pull latest images
    docker pull vhiveease/integ_test_runner:ubuntu20base
    docker pull vhiveease/cri_test_runner

    if [ "$RESTART_FLAG" == "restart" ]; then
        kind get clusters | while read line ; do kind delete cluster --name "$line" ; done
    fi
    for number in $(seq 1 $NUM_OF_RUNNERS)
    do
        kind create cluster --image vhiveease/cri_test_runner --name "cri-test-github-runner-${HOSTNAME}-${number}"
        sleep 2m
        docker exec -it \
            -e RUNNER_ALLOW_RUNASROOT=1 \
            -w /root/actions-runner \
            "cri-test-github-runner-${HOSTNAME}-${number}-control-plane" \
            ./config.sh \
                --url "${_SHORT_URL}" \
                --token "${RUNNER_TOKEN}" \
                --name "cri-test-github-runner-${HOSTNAME}-${number}" \
                --work "/root/_work" \
                --labels "cri" \
                --unattended \
                --replace
        sleep 20s
        docker exec -it \
            "cri-test-github-runner-${HOSTNAME}-${number}-control-plane" \
            systemctl daemon-reload
        docker exec -it \
            "cri-test-github-runner-${HOSTNAME}-${number}-control-plane" \
            systemctl enable connect_github_runner --now
    done
    ;;
"gvisor-cri")
    if [ "$RESTART_FLAG" == "restart" ]; then
    	sudo multipass delete --all
	sudo multipass purge
	sleep 2s
    fi

    for number in $(seq 1 $NUM_OF_RUNNERS)
    do
	vm_name="gv-vm-${number}"
        
	sudo multipass launch \
            --name "${vm_name}" \
	    --cpus 4 \
	    --mem 4G \
	    --disk 16G 20.04 <<< "no"
	sleep 2s

	RUNNER_NAME=${vm_name}
	SANDBOX="gvisor"

	export REPO_NAME=$REPO_NAME SANDBOX=$SANDBOX RUNNER_NAME=$RUNNER_NAME RUNNER_TOKEN=$RUNNER_TOKEN RUNNER_LABEL=$RUNNER_LABEL
	sudo multipass exec "${vm_name}" -- bash -s <<< `envsubst < ${PWD}/start_bare-metal_runner.sh`
    done
    ;;
*)
    echo "Invalid label"
    ;;
esac
