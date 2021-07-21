#!/bin/bash

# MIT License
#
# Copyright (c) 2021 Mert Bora Alper and EASE lab
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

# Execute this script (on the local) as
#   GH_ACCESS_TOKEN=... ./easy-recreate.sh <HOSTNAME> <N> [USER]
#

set -x
set -e

PWD="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"

REMOTE=$1
COUNT=$2
AS_USER=${3:-`whoami`}

sudo apt update -qq
sudo apt install software-properties-common
sudo add-apt-repository --yes --update ppa:ansible/ansible
sudo apt install -q ansible


ansible-playbook -u $AS_USER -i ${REMOTE}, ${PWD}/delete-runners.yaml

for i in $(seq ${COUNT}); do
    ansible-playbook -u $AS_USER -i ${REMOTE}, ${PWD}/create-runner.yaml
done
