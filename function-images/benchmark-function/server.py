#1626122012.2109742

import grpc
import time
import json
import logging
from concurrent import futures
import helloworld_pb2
import helloworld_pb2_grpc
class Greeter(helloworld_pb2_grpc.GreeterServicer):
    def SayHello(self, request, context):
        userinput = json.loads(request.name)
        targettime = userinput['executiontime']
        timeout = time.time() + targettime * 0.001
        print(str(timeout))
        while time.time() < timeout:
            dummy = 1 + 1
        return helloworld_pb2.HelloReply(message='testing')
def serve():
    server = grpc.server(futures.ThreadPoolExecutor(max_workers=1))
    print('hello1')
    helloworld_pb2_grpc.add_GreeterServicer_to_server(Greeter(), server)
    print('hello2')
    server.add_insecure_port('[::]:50051')
    print('hello3')
    server.start()
    print('hello4')
    server.wait_for_termination()
    print('terminated')
if __name__ == '__main__':
    logging.basicConfig()
