from concurrent import futures
import logging

import grpc

import random
import string
import pyaes

import helloworld_pb2
import helloworld_pb2_grpc

def generate(length):
    letters = string.ascii_lowercase + string.digits
    return ''.join(random.choice(letters) for i in range(length))

KEY = b'\xa1\xf6%\x8c\x87}_\xcd\x89dHE8\xbf\xc9,'
message = generate(100)
message2 = generate(100)

responses = ["record_response", "replay_response"]

class Greeter(helloworld_pb2_grpc.GreeterServicer):

    def SayHello(self, request, context):
        aes = pyaes.AESModeOfOperationCTR(KEY)

        if request.name == "record":
            msg = 'Hello, %s!' % responses[0]
            ciphertext = aes.encrypt(message)
        elif request.name == "replay":
            msg = 'Hello, %s!' % responses[1]
            ciphertext = aes.encrypt(message2)
        else:
            msg = 'Hello, %s!' % request.name
            ciphertext = aes.encrypt(message)


        return helloworld_pb2.HelloReply(message=msg)


def serve():
    server = grpc.server(futures.ThreadPoolExecutor(max_workers=1))
    helloworld_pb2_grpc.add_GreeterServicer_to_server(Greeter(), server)
    server.add_insecure_port('[::]:50051')
    server.start()
    server.wait_for_termination()


if __name__ == '__main__':
    logging.basicConfig()
    serve()
