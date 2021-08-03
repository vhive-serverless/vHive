import logging

import grpc

import helloworld_pb2
import helloworld_pb2_grpc
import json
import sys
from minio import Minio

def run():
    with grpc.insecure_channel('localhost:50051') as channel:
        stub = helloworld_pb2_grpc.GreeterStub(channel)

        minioClient = Minio("10.138.0.27:9000",
                            access_key="minioadmin",
                            secret_key="minioadmin",
                            secure=False)

        input_hash = (minioClient.get_object("mybucket", "hello_world/hello")
                                .data.decode('utf-8').splitlines())#[1]
        print(input_hash)
        name = json.dumps({
            'input_hash': input_hash,
            'thunks': 'not sure yet',
            'storageBackend': 'http://10.138.0.27:9000',
            'timelog': False
            })
        return 0

        response = stub.SayHello(helloworld_pb2.HelloRequest(name=name))
    print("Greeter client received: " + response.message)


if __name__ == '__main__':
    logging.basicConfig()
    run()
