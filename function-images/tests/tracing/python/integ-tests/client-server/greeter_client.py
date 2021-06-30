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

from __future__ import print_function

import sys
import os
# adding python tracing sources to the system path
sys.path.insert(0, os.getcwd() + '/../../../../../../utils/tracing/python')
import tracing

import time
import logging

import grpc

import helloworld_pb2
import helloworld_pb2_grpc

import argparse

parser = argparse.ArgumentParser()
parser.add_argument("-server", "--server", dest = "url", default = "localhost:50051", help="Server url and port")
parser.add_argument("-zipkin", "--zipkin", dest = "zipkin", default = "http://localhost:9411/api/v2/spans", help="Zipkin endpoint url")
args = parser.parse_args()

print("client using url: "+args.url)

def run():
    time.sleep(10)
    tracing.initTracer("client", url=args.zipkin)
    tracing.grpcInstrumentClient()

    with tracing.Span("test span"):
        with grpc.insecure_channel(args.url) as channel:
            stub = helloworld_pb2_grpc.GreeterStub(channel)
            response = stub.SayHello(helloworld_pb2.HelloRequest(name="world"))
        with tracing.Span("test child span"):
            print("Greeter client received: " + response.message)


if __name__ == '__main__':
    logging.basicConfig()
    run()
