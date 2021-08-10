# MIT License
#
# Copyright (c) 2021 Michal Baczun and EASE lab
#
# Permission is hereby granted, free of charge, to any person obtaining a copy
# of this software and associated documentation files (the "Software"), to deal
# in the Software without restriction, including without limitation the rights
# to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
# copies of the Software, and to permit persons to whom the Software is
# furnished to do so, subject to the following conditions:
#
# The above copyright notice and this permission notice shall be included in all
# copies or substantial portions of the Software.
#
# THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
# IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
# FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
# AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
# LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
# OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
# SOFTWARE.

from __future__ import print_function

import sys
import os

# adding python tracing sources to the system path
sys.path.insert(0, os.getcwd() + '/../proto/')
sys.path.insert(0, os.getcwd() + '/../../../../utils/tracing/python')
import tracing
import helloworld_pb2_grpc
import helloworld_pb2
import mapreduce_pb2_grpc
import mapreduce_pb2
import destination as XDTdst
import source as XDTsrc
import utils as XDTutil

import grpc
from grpc_reflection.v1alpha import reflection
import argparse
import boto3
import logging as log
import socket
import json
import resource
from io import StringIO ## for Python 3
import time
from joblib import Parallel, delayed

import pickle

from concurrent import futures

parser = argparse.ArgumentParser()
parser.add_argument("-dockerCompose", "--dockerCompose", dest="dockerCompose", default=False, help="Env docker compose")
parser.add_argument("-mAddr", "--mAddr", dest="mAddr", default="mapper.default.192.168.1.240.sslip.io:80",
                    help="trainer address")
parser.add_argument("-rAddr", "--rAddr", dest="rAddr", default="reducer.default.192.168.1.240.sslip.io:80",
                    help="reducer address")
parser.add_argument("-sp", "--sp", dest="sp", default="80", help="serve port")
parser.add_argument("-zipkin", "--zipkin", dest="zipkinURL",
                    default="http://zipkin.istio-system.svc.cluster.local:9411/api/v2/spans",
                    help="Zipkin endpoint url")

args = parser.parse_args()

if tracing.IsTracingEnabled():
    tracing.initTracer("driver", url=args.zipkinURL)
    tracing.grpcInstrumentClient()
    tracing.grpcInstrumentServer()

# constants
INPUT_MAPPER_PREFIX = "artemiy/"
OUTPUT_MAPPER_PREFIX = "artemiy/task/mapper/"
INPUT_REDUCER_PREFIX = OUTPUT_MAPPER_PREFIX
OUTPUT_REDUCER_PREFIX = "artemiy/task/reducer/"
S3 = "S3"
XDT = "XDT"
NUM_MAPPERS = int(os.getenv('NUM_MAPPERS', "4"))  # can't be more than 2215 
NUM_REDUCERS = int(os.getenv('NUM_REDUCERS', "2")) # must be power of 2 and smaller than NUM_MAPPERS 

# set aws credentials:
AWS_ID = os.getenv('AWS_ACCESS_KEY', "")
AWS_SECRET = os.getenv('AWS_SECRET_KEY', "")


class GreeterServicer(helloworld_pb2_grpc.GreeterServicer):
    def __init__(self, transferType, XDTconfig=None):
        self.transferType = transferType
        if transferType == S3:
            self.s3_client = boto3.resource(
                service_name='s3',
                region_name=os.getenv("AWS_REGION", 'us-west-1'),
                aws_access_key_id=AWS_ID,
                aws_secret_access_key=AWS_SECRET
            )
        elif transferType == XDT:
            if XDTconfig is None:
                log.fatal("Empty XDT config")
            self.XDTclient = XDTsrc.XDTclient(XDTconfig)
            self.XDTconfig = XDTconfig

    def put(self, bucket, key, obj, metadata=None):
        msg = "Driver uploading object with key '" + key + "' to " + self.transferType
        log.info(msg)
        with tracing.Span(msg):
            # pickled = pickle.dumps(obj)
            if self.transferType == S3:
                s3object = self.s3_client.Object(bucket_name=bucket, key=key)
                if metadata is None:
                    s3object.put(Body=obj)
                else:
                    s3object.put(Body=obj, Metadata=metadata)
            elif self.transferType == XDT:
                key = self.XDTclient.BroadcastPut(payload=obj)

        return key

    def get(self, key):
        msg = "Driver gets key '" + key + "' from " + self.transferType
        log.info(msg)
        with tracing.Span(msg):
            response = None
            if self.transferType == S3:
                obj = self.s3_client.Object(bucket_name=self.benchName, key=key)
                response = obj.get()
            elif self.transferType == XDT:
                return XDTdst.BroadcastGet(key, self.XDTconfig)

        return response['Body'].read()

    def call_mapper(self, arg: dict):
        #   "srcBucket": "storage-module-test", 
        # "destBucket": "storage-module-test", 
        # "keys": ["part-00000"],
        # "jobId": "0",
        # "mapperId": 0,
        # "nReducers": NUM_REDUCERS

        log.info(f"Invoking Mapper {arg['mapperId']}")
        channel = grpc.insecure_channel(args.mAddr)
        stub = mapreduce_pb2_grpc.MapperStub(channel)

        req = mapreduce_pb2.MapRequest(
            srcBucket = arg["srcBucket"],
            destBucket = arg["destBucket"],
            #keys =,
            jobId = arg["jobId"],
            mapperId = arg["mapperId"],
            nReducers = arg["nReducers"],
        )
        for key_string in arg["keys"]:
            grpc_keys = mapreduce_pb2.Keys()
            grpc_keys.key = key_string
            req.keys.append(grpc_keys)

        resp = stub.Map(req)
        log.info(f"mapper reply: {resp}")
        return resp.keys

    def call_reducer(self, arg: dict):
        # "srcBucket": "storage-module-test", 
        # "destBucket": "storage-module-test",
        # "keys": reduce_input_keys,
        # "nReducers": NUM_REDUCERS,
        # "jobId": "0",
        # "reducerId": 0, 
        log.info(f"Invoking Reducer {arg['reducerId']}")
        channel = grpc.insecure_channel(args.rAddr)
        stub = mapreduce_pb2_grpc.ReducerStub(channel)

        req = mapreduce_pb2.ReduceRequest(
            srcBucket = arg["srcBucket"],
            destBucket = arg["destBucket"],
            #keys =,
            jobId = arg["jobId"],
            reducerId = arg["reducerId"],
            nReducers = arg["nReducers"],
        )
        for key_string in arg["keys"]:
            grpc_keys = mapreduce_pb2.Keys()
            grpc_keys.key = key_string
            req.keys.append(grpc_keys)

        resp = stub.Reduce(req)
        log.info(f"reducer reply: {resp}")

    # Driver code below
    def SayHello(self, request, context):
        log.info("Driver received a request")

        map_ev = {
        "srcBucket": "storage-module-test", 
        "destBucket": "storage-module-test",
        "keys": ["part-00000"],
        "jobId": "0",
        "mapperId": 0,
        "nReducers": NUM_REDUCERS,
            }
        map_tasks = [] 
        for i in range(NUM_MAPPERS):
            map_tasks.append(map_ev.copy())
            map_tasks[i]['keys'] = ["part-" + str(i).zfill(5)]
            map_tasks[i]['mapperId'] = i

        # for task in map_tasks:
        #     self.call_mapper(task)
        mapper_responses=[]
        ex = futures.ThreadPoolExecutor(max_workers=NUM_MAPPERS)
        all_result_futures = ex.map(self.call_mapper, map_tasks)

        reduce_input_keys = {}
        for i in range(NUM_REDUCERS):
            reduce_input_keys[i] = []

        for result_keys in all_result_futures: #this is just to wait for all futures to complete
            for i in range(NUM_REDUCERS):
                reduce_input_keys[i].append(result_keys[i].key)

        log.info("calling mappers done")
        # print(mapper_responses)

        # reduce_input_keys = ["map_" + str(x) for x in range(NUM_MAPPERS)] # this list is the same for
                                                                        # all reducers as each of them has to read 
                                                                        # a shuffle result from each mapper
        reduce_ev = {
            "srcBucket": "storage-module-test", 
            "destBucket": "storage-module-test",
            "nReducers": NUM_REDUCERS,
            "jobId": "0",
            "reducerId": 0, 
            }
        reducer_tasks = [] 
        for i in range(NUM_REDUCERS):
            log.info("assigning keys to reducer %d", i)
            log.info(reduce_input_keys[i])
            reduce_ev["keys"] = reduce_input_keys[i]
            reducer_tasks.append(reduce_ev.copy())
            reducer_tasks[i]['reducerId'] = i

        # for task in reducer_tasks:
        #     self.call_reducer(task)
        reducer_responses=[]
        ex = futures.ThreadPoolExecutor(max_workers=NUM_REDUCERS)
        all_result_futures = ex.map(self.call_reducer, reducer_tasks)
        for result in all_result_futures:
            reducer_responses.append(result)
        log.info("calling reducers done")
        #print(reducer_responses)

        return helloworld_pb2.HelloReply(message="jobs done")


def serve():
    transferType = os.getenv('TRANSFER_TYPE', S3)

    XDTconfig = dict()
    if transferType == XDT:
        XDTconfig = XDTutil.loadConfig()
        log.info("XDT config:")
        log.info(XDTconfig)

    log.info("Using inline or s3 transfers")
    max_workers = int(os.getenv("MAX_SERVER_THREADS", 16))
    server = grpc.server(futures.ThreadPoolExecutor(max_workers=max_workers))
    helloworld_pb2_grpc.add_GreeterServicer_to_server(
        GreeterServicer(transferType=transferType, XDTconfig=XDTconfig), server)
    SERVICE_NAMES = (
        helloworld_pb2.DESCRIPTOR.services_by_name['Greeter'].full_name,
        reflection.SERVICE_NAME,
    )
    reflection.enable_server_reflection(SERVICE_NAMES, server)
    server.add_insecure_port('[::]:' + args.sp)
    server.start()
    server.wait_for_termination()


if __name__ == '__main__':
    log.basicConfig(level=log.INFO)
    serve()
