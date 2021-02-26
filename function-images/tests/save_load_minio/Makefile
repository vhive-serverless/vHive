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

all: local push_docker

.PHONY: proto_gen
proto_gen:
	GO111MODULE="on" protoc proto/save_load.proto \
	    --proto_path=./proto \
	    --go_out=./proto_gen --go-grpc_out=./proto_gen \
	    --go_opt=paths=source_relative --go-grpc_opt=paths=source_relative

local: proto_gen build_local

build_local:
	mkdir -p bins
	cd client && go build -o ../bins/client
	cd server && go build -o ../bins/server

push_docker: build_docker
	docker tag save_load_minio_test:latest vhiveease/save_load_minio_test:latest
	docker push vhiveease/save_load_minio_test:latest

build_docker: proto_gen
	docker build -t save_load_minio_test:latest .

clean:
	rm -rf bins
