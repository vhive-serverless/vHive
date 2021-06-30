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

ROOT = ../../../../../../

all-image: server-image client-image go-client-image

server-image: Dockerfile server.py helloworld_pb2_grpc.py helloworld_pb2.py
	cd $(ROOT) && docker build --tag vhiveease/py-tracing-server:latest --target server -f ./function-images/tests/tracing/python/integ-tests/client-server/Dockerfile .

client-image: Dockerfile greeter_client.py helloworld_pb2_grpc.py helloworld_pb2.py
	cd $(ROOT)  && docker build --tag vhiveease/py-tracing-client:latest --target client -f ./function-images/tests/tracing/python/integ-tests/client-server/Dockerfile .

go-client-image: goclient/Dockerfile goclient/goclient.go goclient/go.mod goclient/go.sum
	cd $(ROOT)  && docker build --tag vhiveease/py-tracing-go-client:latest -f ./function-images/tests/tracing/python/integ-tests/client-server/goclient/Dockerfile .