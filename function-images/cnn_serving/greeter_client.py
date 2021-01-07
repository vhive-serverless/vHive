from __future__ import print_function
import logging

import grpc

import helloworld_pb2
import helloworld_pb2_grpc

import sys

def run():
    with grpc.insecure_channel('localhost:50051') as channel:
        stub = helloworld_pb2_grpc.GreeterStub(channel)

        name = 'you'

        if len(sys.argv) == 2:
            if sys.argv[1] == 'rec':
                name = 'record'
            elif sys.argv[1] == 'rep':
                name = 'replay'
            else:
                exit(-1)

        response = stub.SayHello(helloworld_pb2.HelloRequest(name=name))
    print("Greeter client received: " + response.message)


if __name__ == '__main__':
    logging.basicConfig()
    run()
