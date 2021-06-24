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
.PHONY: all clean image-* push-*


all: bin/consumer bin/producer

image-all: image-consumer image-producer

push-all: push-consumer push-producer

clean:
	rm -rf bin producer/*.pb.go proto/*.pb.go


bin/producer: producer/* producer/helloworld_grpc.pb.go producer/helloworld.pb.go
	go build -o bin/producer eventing/producer

TAG_PRODUCER = vhiveease/chained-functions-eventing-producer:latest
image-producer: Dockerfile bin/producer
	docker build --tag ${TAG_PRODUCER} --build-arg target_arg=producer -f ./Dockerfile ../../..

push-producer: image-producer
	docker push ${TAG_PRODUCER}


bin/consumer: consumer/*
	go build -o bin/consumer eventing/consumer

TAG_CONSUMER = vhiveease/chained-functions-eventing-consumer:latest
image-consumer: Dockerfile bin/consumer
	docker build --tag ${TAG_CONSUMER} --build-arg target_arg=consumer -f ./Dockerfile ../../..

push-consumer: image-consumer
	docker push ${TAG_CONSUMER}


producer/helloworld_grpc.pb.go producer/helloworld.pb.go:
	protoc -I=../../springboot/proto \
		--go_out=producer \
		--go_opt="paths=source_relative" \
		--go-grpc_out=producer \
		--go-grpc_opt="paths=source_relative" \
		helloworld.proto
	# A kludge since I could not set protoc to generate outputs in `producer/` under
	# package main.
	sed -i 's/package helloworld/package main/g' producer/*.pb.go
