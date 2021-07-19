# MIT License
#
# Copyright (c) 2021 Michal Baczun and EASE lab
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
HERE = ./function-images/tests/video-bench

all: all-image 

all-image: decoder-image recog-image streaming-image

all-image-push: decoder-image-push recog-image-push streaming-image-push

decoder-image: Dockerfile proto/videoservice_pb2_grpc.py proto/videoservice_pb2.py decoder/decode.py
	cd $(ROOT) && docker build -t vhiveease/video-bench-decoder --target decode -f $(HERE)/Dockerfile .

recog-image: Dockerfile proto/videoservice_pb2_grpc.py proto/videoservice_pb2.py object_recognition/recog.py object_recognition/imagenet_labels.txt
	cd $(ROOT) && docker build -t vhiveease/video-bench-recog --target recog -f $(HERE)/Dockerfile .

streaming-image: Dockerfile proto/videoservice_grpc.pb.go proto/videoservice.pb.go video_streaming/video-streaming.go video_streaming/reference/video.mp4
	docker build -t vhiveease/video-bench-streaming --target streaming .

decoder-image-push: decoder-image
	docker push vhiveease/video-bench-decoder:latest

recog-image-push: recog-image
	docker push vhiveease/video-bench-recog:latest

streaming-image-push: streaming-image
	docker push vhiveease/video-bench-streaming:latest

proto: proto/videoservice.proto
	protoc --go_out=. --go_opt=paths=source_relative --go-grpc_out=. --go-grpc_opt=paths=source_relative proto/videoservice.proto && cd proto && python -m grpc_tools.protoc -I. --python_out=. --grpc_python_out=. videoservice.proto