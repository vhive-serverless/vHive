import grpc
import time
import json
import logging
import os
import pickle

from concurrent import futures
from minio.commonconfig import CopySource
from minio import Minio

import helloworld_pb2
import helloworld_pb2_grpc

class Greeter(helloworld_pb2_grpc.GreeterServicer):
    
    def SayHello(self, request, context):
        
        msg = ''
        print('received: ' + request.name)
        userinput = json.loads(request.name)
        
        if 'objectsize' in userinput:
            
            initialtime = time.time()
            targetsize = userinput['objectsize']
            print('Fetching an object of size ' + str(targetsize) +' bytes')
            client = Minio("10.138.0.34:9000", access_key="minioadmin", secret_key="minioadmin", secure=False)
            buckets = client.list_buckets()
            objectname = ''
            for bucket in buckets:
                
                if bucket.name == 'mybucket':
                    objects = client.list_objects(bucket.name)
                    for objs in objects:
                        data = client.stat_object(bucket.name, objs.object_name)
                        if data.size == targetsize/1000*1024:
                            objectname = objs.object_name
                            print('desired object found: '+objectname)
                            break
                            
            if objectname == '':
                msg = 'object of desired size does not exist in the bucket.'
                print('object not found')
                return helloworld_pb2.HelloReply(message = msg)
            
            obj = client.fget_object('mybucket', objectname, 'mybucket'+objectname)
            with open("/tmp/" + objectname, "wb") as tmpfile:
                pickle.dump(obj, tmpfile)
                print('saved to //tmp directory')
                
            elapsedtime = time.time()-initialtime
            msg = msg + 'Objectsize benchmark completed for ' + str(targetsize) +' bytes. File stored in \\tmp, used '+ str(elapsedtime)+' miliseconds.\n'

        if 'executiontime' in userinput:

            targettime = userinput['executiontime']
            print('waiting for ' + str(targettime) +' miliseconds')
            timeout = time.time() + targettime * 0.001
            while time.time() < timeout:

                dummyoperation = 1 + 1

            msg = msg + 'Executiontime benchmark completed for ' + str(targettime) + 'miliseconds. Terminated at: ' + str(timeout) + '\n'
            print(msg)

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
