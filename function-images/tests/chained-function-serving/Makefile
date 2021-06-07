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

all: all-image 

all-image: producer-image consumer-image

all-image-push: producer-image-push consumer-image-push

clean:
	rm -f ./producer-bin ./consumer-bin


producer-image: Dockerfile producer/producer.go proto/prodcon.pb.go proto/prodcon_grpc.pb.go
	docker build --tag vhiveease/chained-functions-serving-producer:latest --build-arg target_arg=producer .

producer-image-push: producer-image
	docker push vhiveease/chained-functions-serving-producer:latest


consumer-image: Dockerfile consumer/consumer.go proto/prodcon.pb.go proto/prodcon_grpc.pb.go
	docker build --tag vhiveease/chained-functions-serving-consumer:latest --build-arg target_arg=consumer .

consumer-image-push: consumer-image
	docker push vhiveease/chained-functions-serving-consumer:latest


cmd/server/tempReader_grpc.pb.go cmd/server/tempReader.pb.go: cmd/server/tempReader.proto
	protoc \
		--go_out=. \
		--go_opt="paths=source_relative" \
		--go-grpc_out=. \
		--go-grpc_opt="paths=source_relative" \
		cmd/server/tempReader.proto