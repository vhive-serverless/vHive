#!/bin/bash

# MIT License
#
# Copyright (c) 2021 Dmitrii Ustiugov and EASE lab
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

set -e

DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null 2>&1 && pwd )"

sudo cp $DIR/../../../minio_scripts/mc /usr/local/bin

echo Preparing a folder for MinIO local storage
sudo mkdir -p /minio-storage

echo Deploying MinIO in k8s
pushd $DIR/../../../../configs/storage/minio > /dev/null
MINIO_NODE_NAME=$HOSTNAME MINIO_PATH=/minio-storage envsubst < pv.yaml | kubectl apply -f -
kubectl apply -f pv-claim.yaml
kubectl apply -f deployment.yaml
kubectl apply -f service.yaml
popd > /dev/null

sleep 10

echo Making the bucket
mc alias set myminio http://10.96.0.46:9000 minio minio123
sleep 10
mc mb myminio/mybucket

echo Deploying the gRPC server as a function
kn service apply \
    minio_test -f $DIR/../../../configs/knative_workloads/tests/minio.yaml \
    --concurrency-target 1 --scale=1

$DIR/../bins/client -d \
    --addr="minio-test.default.192.168.1.240.xip.io:80" \
    --minioAddr="10.96.0.46:9000" -size=1024
