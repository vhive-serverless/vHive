from __future__ import print_function
import logging

import grpc

import helloworld_pb2
import helloworld_pb2_grpc

import json
import sys

def run():
    with grpc.insecure_channel('localhost:50051') as channel:
        stub = helloworld_pb2_grpc.GreeterStub(channel)

        name = json.dumps({
            'input_hash': 'dummy',
            'thunks': 'dummy',
            'storageBackend': 'dummy',
            'timelog': False
        })

        response = stub.SayHello(helloworld_pb2.HelloRequest(name=name))
    print("Greeter client received: " + response.message)


if __name__ == '__main__':
    logging.basicConfig()
    run()
