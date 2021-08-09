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

ROOT = ../../../
HERE = ./function-images/tests/map-reduce

PLATFORM = --platform linux/amd64
MODE = --load

# For debugging on Apple M1 below
#PLATFORM = --platform linux/arm64
#MODE = --push

.PHONY: proto

all: all-image 

all-image: driver-image mapper-image reducer-image

all-image-push: driver-image-push mapper-image-push reducer-image-push

driver-image: Dockerfile proto/mapreduce_pb2_grpc.py proto/mapreduce_pb2.py driver/main.py
	docker buildx build $(PLATFORM) \
	-t vhiveease/mapreduce-driver:latest \
	--build-arg target_arg=driver \
	--secret id=GOPRIVATE_KEY \
	-f Dockerfile \
	$(ROOT) $(MODE)

mapper-image: Dockerfile proto/mapreduce_pb2_grpc.py proto/mapreduce_pb2.py mapper/main.py
	docker buildx build $(PLATFORM) \
	-t vhiveease/mapreduce-mapper:latest \
	--build-arg target_arg=mapper \
	--secret id=GOPRIVATE_KEY \
	-f Dockerfile \
	$(ROOT) $(MODE)

reducer-image: Dockerfile proto/mapreduce_pb2_grpc.py proto/mapreduce_pb2.py reducer/main.py
	docker buildx build $(PLATFORM) \
	-t vhiveease/mapreduce-reducer:latest \
	--build-arg target_arg=reducer \
	--secret id=GOPRIVATE_KEY \
	-f Dockerfile \
	$(ROOT) $(MODE)

driver-image-push: driver-image
	docker push vhiveease/mapreduce-driver:latest

mapper-image-push: mapper-image
	docker push vhiveease/mapreduce-mapper:latest

reducer-image-push: reducer-image
	docker push vhiveease/mapreduce-reducer:latest

proto: proto/mapreduce.proto proto/helloworld.proto
	python -m grpc_tools.protoc -I./proto --python_out=./proto --grpc_python_out=./proto ./proto/mapreduce.proto
	python -m grpc_tools.protoc -I./proto --python_out=./proto --grpc_python_out=./proto ./proto/helloworld.proto
