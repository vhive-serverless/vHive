import grpc
import time
import json
import logging
from concurrent import futures
import helloworld_pb2
import helloworld_pb2_grpc

class Greeter(helloworld_pb2_grpc.GreeterServicer):
    
    def SayHello(self, request, context):
        
 
        
        print('received: ' + request.name)
        userinput = json.loads(request.name)
        
        if 'objectsize' in userinput:

            initialtime = time.time()
            targetsize = userinput['objectsize']
            print('fetching an object of size' + str(targetsize) +'bytes')
            client = Minio("10.138.0.34:9000", access_key="minioadmin", secret_key="minioadmin", secure=False)
            print('Minio is fine')

            buckets = client.list_buckets()
            print('check0')
            objectname = 'tba'
            for bucket in buckets:
#                print(bucket.name, bucket.creation_date)                                                                                                                                           
#                print(client.stat_object(bucket.name, 'client.go'))                                                                                                                                

                objects = client.list_objects(bucket.name)
                print('check0.5')
                print(str(objects))
                print('why wony you print')
                count = 0

                for objs in objects:
                    print(str(objs))
                    print('object^')
                    count = count + 1
                    print(count)
                    
                    
                    
                for objs in objects:
                    print('check0.6')
                    data = client.stat_object(bucket.name, objs.object_name)
                    print('check0.7')
                    print(data.size)
                    if data.size == targetsize:
                        objectname = objs.object_name
                        print(objectname)
                        break
                        
            obj = client.fget_object('mybucket', objectname, '/home/yalew/vhive/function-images/minio_scripts/myminio/mybucket/'+objectname)
            print('check1')
            new_file = open("/tmp/client.go", "a")
            new_file.write(str(obj))
            finaltime = time.time()
            elapsedtime = finaltime-initialtime
            msg = 'stored in tmp, used '+ str(elapsedtime)+'miliseconds'
            return helloworld_pb2.HelloReply(message=msg)
        
        
        if 'executiontime' in userinput:
            targettime = userinput['executiontime']
            print('waiting for ' + str(targettime) +' miliseconds')
            timeout = time.time() + targettime * 0.001
            while time.time() < timeout:
                dummyoperation = 1 + 1
            msg = 'Terminated at: ' + str(timeout)
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
