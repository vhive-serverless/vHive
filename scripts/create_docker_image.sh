#!/bin/bash

git clone -b upf_master --recurse-submodules https://github.com/ustiugov/firecracker-containerd

docker build -t vhiveease/fcuvm_dev:latest -f firecracker-containerd/_submodules/firecracker/tools/devctr/Dockerfile.x86_64 firecracker-containerd/_submodules/firecracker/

rm -rf firecracker-containerd

docker login -u vhiveease

docker push vhiveease/fcuvm_dev:latest
