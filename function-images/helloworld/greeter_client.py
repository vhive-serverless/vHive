from __future__ import print_function
import logging
import argparse

import grpc

import helloworld_pb2
import helloworld_pb2_grpc

import sys

def run(target):
    print("target: ", target)
    with grpc.insecure_channel(target) as channel:
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


def get_target(server, port):
    prefix = 'http://'
    server = server[len(prefix):] if server.startswith(prefix) else server
    # return f'{server}:{port}'
    return server + ":" + str(port)

if __name__ == '__main__':
    parser = argparse.ArgumentParser()
    parser.add_argument('-s', '--server', type=str, default='localhost')
    parser.add_argument('-p', '--port', type=int, default=50051)
    args = parser.parse_args()
    logging.basicConfig()
    run(get_target(args.server, args.port))
