#!/bin/bash

# MIT License
#
# Copyright (c) 2023 Siyang Shao and vHive community
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

# Install NVIDIA Drivers
sudo apt-get install linux-headers-$(uname -r)
distribution=$(. /etc/os-release;echo $ID$VERSION_ID | sed -e 's/\.//g') 
CUDA_VERSION="530.30.02-1"
wget https://developer.download.nvidia.com/compute/cuda/repos/$distribution/x86_64/cuda-$distribution.pin \
   && sudo mv cuda-$distribution.pin /etc/apt/preferences.d/cuda-repository-pin-600

sudo apt-key adv --fetch-keys https://developer.download.nvidia.com/compute/cuda/repos/$distribution/x86_64/3bf863cc.pub \
   && echo "deb http://developer.download.nvidia.com/compute/cuda/repos/$distribution/x86_64 /" | sudo tee /etc/apt/sources.list.d/cuda.list

sudo apt-get update \
   && sudo apt-get install -y cuda-drivers=$CUDA_VERSION --no-install-recommends

# Install NVIDIA Container Toolkit
DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null 2>&1 && pwd )"
ROOT="$( cd $DIR && cd .. && cd .. && pwd)"
CONFIGS=$ROOT/configs

# Note: NVIDIA Container TOolkit and NVIDIA Drivers use different distribution format
distribution=$(. /etc/os-release;echo $ID$VERSION_ID) 
NVIDIA_CONTAINER_VERSION=3.13.0-1
curl -s -L https://nvidia.github.io/nvidia-docker/gpgkey | sudo apt-key add - \
   && curl -s -L https://nvidia.github.io/nvidia-docker/$distribution/nvidia-docker.list | sudo tee /etc/apt/sources.list.d/nvidia-docker.list

sudo apt-get update \
   && sudo apt-get install -y nvidia-container-runtime=$NVIDIA_CONTAINER_VERSION

# Note: The config file here is the default config file of containerd with modification based on
# https://docs.nvidia.com/datacenter/cloud-native/kubernetes/install-k8s.html#install-nvidia-container-toolkit-nvidia-docker2
sudo mkdir -p /etc/containerd && sudo cp -b $CONFIGS/gpu/containerd_config.toml /etc/containerd/config.toml