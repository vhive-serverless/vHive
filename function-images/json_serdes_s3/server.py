# Copyright 2015 gRPC authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
"""The Python implementation of the GRPC helloworld.Greeter server."""

from concurrent import futures
import logging
import os
import grpc

import helloworld_pb2
import helloworld_pb2_grpc

from minio import Minio
import json

minioEnvKey = "MINIO_ADDRESS"
data_name = '2.json'
data2_name = '1.json'
data_path = '/pulled_' + data_name
data2_path = '/pulled_' + data2_name

responses = ["record_response", "replay_response"]

minioAddress = os.getenv(minioEnvKey)

class Greeter(helloworld_pb2_grpc.GreeterServicer):

    def SayHello(self, request, context):
        if minioAddress == None:
            return None

        minioClient = Minio(minioAddress,
                access_key='minioadmin',
                secret_key='minioadmin',
                secure=False)
        if request.name == "record":
            msg = 'Hello, %s!' % responses[0]
            minioClient.fget_object('mybucket', data_name, data_path)
            data = open(data_path).read()
            json_data = json.loads(data)
            str_json = json.dumps(json_data, indent=4)
        elif request.name == "replay":
            msg = 'Hello, %s!' % responses[1]
            minioClient.fget_object('mybucket', data2_name, data2_path)
            data2 = open(data2_path).read()
            json_data = json.loads(data2)
            str_json = json.dumps(json_data, indent=4)
        else:
            msg = 'Hello, %s!' % request.name
            minioClient.fget_object('mybucket', data_name, data_path)
            data = open(data_path).read()
            json_data = json.loads(data)
            str_json = json.dumps(json_data, indent=4)

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
