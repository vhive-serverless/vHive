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
mkdir -p /tmp/save_load_minio

echo Building the client and the server locally
pushd $DIR/../ > /dev/null
make all
popd > /dev/null

echo Deploying a MinIO server
docker run -d -p 50052:9000 --name minio \
  -e "MINIO_ROOT_USER=minio" \
  -e "MINIO_ROOT_PASSWORD=minio123" \
  minio/minio server /data

sleep 5

echo Making the bucket
mc alias set myminio http://localhost:50052 minio minio123
mc mb myminio/mybucket

echo Running the gRPC server
docker run -m=256m --memory-swap=256m  -d --net=host --name=gServer -v /tmp/save_load_minio:/tmp vhiveease/save_load_minio_test:latest

$DIR/../bins/client -d -size=1024

echo Running the client
docker stop minio gServer
docker rm minio gServer
