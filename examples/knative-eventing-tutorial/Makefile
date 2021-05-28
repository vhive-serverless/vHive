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

all: server temp-consumer overheat-consumer

all-image: server-image temp-consumer-image overheat-consumer-image

all-image-push: server-image-push overheat-consumer-image-push temp-consumer-image-push

clean:
	rm -f ./server ./overheat-consumer ./temp-consumer cmd/server/tempReader_grpc.pb.go cmd/server/tempReader.pb.go


server: cmd/server/main.go cmd/server/tempReader_grpc.pb.go cmd/server/tempReader.pb.go
	go build -o ./server pipeline/cmd/server

server-image: Dockerfile cmd/server/main.go cmd/server/tempReader_grpc.pb.go cmd/server/tempReader.pb.go
	docker build --tag vhiveease/ease-pipeline-server:latest --build-arg target_arg=server .

server-image-push: server-image
	docker push vhiveease/ease-pipeline-server:latest


overheat-consumer: cmd/overheatConsumer/main.go cmd/eventschemas.go
	go build -o ./overheat-consumer pipeline/cmd/overheatConsumer

overheat-consumer-image: Dockerfile cmd/overheatConsumer/main.go cmd/eventschemas.go
	docker build --tag vhiveease/ease-pipeline-overheat-consumer:latest --build-arg target_arg=overheat-consumer .

overheat-consumer-image-push: overheat-consumer-image
	docker push vhiveease/ease-pipeline-overheat-consumer:latest


temp-consumer:  cmd/tempConsumer/main.go cmd/eventschemas.go
	go build -o ./temp-consumer pipeline/cmd/tempConsumer

temp-consumer-image: Dockerfile cmd/tempConsumer/main.go cmd/eventschemas.go
	docker build --tag vhiveease/ease-pipeline-temp-consumer:latest --build-arg target_arg=temp-consumer .

temp-consumer-image-push: temp-consumer-image
	docker push vhiveease/ease-pipeline-temp-consumer:latest


cmd/server/tempReader_grpc.pb.go cmd/server/tempReader.pb.go: cmd/server/tempReader.proto
	protoc \
		--go_out=. \
		--go_opt="paths=source_relative" \
		--go-grpc_out=. \
		--go-grpc_opt="paths=source_relative" \
		cmd/server/tempReader.proto
