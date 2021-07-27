import grpc
import json
import logging
import helloworld_pb2
import helloworld_pb2_grpc

def run():
    with grpc.insecure_channel('localhost:50051') as channel:
        
        stub = helloworld_pb2_grpc.GreeterStub(channel)
        
        #userinput = {'executiontime':2000}
        userinput = {'objectsize':10000}
        input_str = json.dumps(userinput)
        print('sending string input: ' + input_str)
        response = stub.SayHello(helloworld_pb2.HelloRequest(name=input_str))
        
    print(response.message)
    
if __name__ == '__main__':
    logging.basicConfig()
    run()
