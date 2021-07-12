import grpc
import json
import logging
import helloworld_pb2
import helloworld_pb2_grpc

def run():
    print('hello1')
    with grpc.insecure_channel('localhost:50051') as channel:
        stub = helloworld_pb2_grpc.GreeterStub(channel)
        print('hello1')
        userinput = {'executiontime':100}
        name = json.dumps(userinput)
        print('hello')
        response = stub.SayHello(helloworld_pb2.HelloRequest(name=name))
        print('huh')
    print('hello')
    
if __name__ == '__main__':
    logging.basicConfig()
    run()
