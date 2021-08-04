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
import tuning_pb2_grpc
import tuning_pb2
import destination as XDTdst
import source as XDTsrc
import utils as XDTutil

import grpc
from grpc_reflection.v1alpha import reflection
import argparse
import boto3
import logging as log
import socket

import sklearn.datasets as datasets
from sklearn.ensemble import RandomForestRegressor
from sklearn.model_selection import  cross_val_predict
from sklearn.metrics import roc_auc_score
import itertools
import numpy as np
import pickle
from sklearn.model_selection import StratifiedShuffleSplit

from concurrent import futures

parser = argparse.ArgumentParser()
parser.add_argument("-dockerCompose", "--dockerCompose", dest="dockerCompose", default=False, help="Env docker compose")
parser.add_argument("-tAddr", "--tAddr", dest="tAddr", default="trainer.default.192.168.1.240.sslip.io:80",
                    help="trainer address")
parser.add_argument("-sp", "--sp", dest="sp", default="80", help="serve port")
parser.add_argument("-zipkin", "--zipkin", dest="zipkinURL",
                    default="http://zipkin.istio-system.svc.cluster.local:9411/api/v2/spans",
                    help="Zipkin endpoint url")

args = parser.parse_args()

if tracing.IsTracingEnabled():
    tracing.initTracer("driver", url=args.zipkinURL)
    tracing.grpcInstrumentClient()
    tracing.grpcInstrumentServer()

INLINE = "INLINE"
S3 = "S3"
XDT = "XDT"

# set aws credentials:
AWS_ID = os.getenv('AWS_ACCESS_KEY', "")
AWS_SECRET = os.getenv('AWS_SECRET_KEY', "")

def get_self_ip():
    s = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
    try:
        # doesn't even have to be reachable
        s.connect(('10.255.255.255', 1))
        IP = s.getsockname()[0]
    except Exception:
        IP = '127.0.0.1'
    finally:
        s.close()
    return IP

def generate_dataset():
	n_samples = 1000
	n_features = 1024 
	X, y = datasets.make_classification(n_samples,
	                                    n_features,
	                                    n_redundant=0,
	                                    n_clusters_per_class=2,
	                                    weights=[0.9, 0.1],
	                                    flip_y=0.1,
	                                    random_state=42)
	return {'features': X, 'labels': y}

def generate_hyperparam_sets(param_config):
	keys = list(param_config.keys())
	values = [param_config[k] for k in keys]

	for elements in itertools.product(*values):
		yield dict(zip(keys, elements))

class GreeterServicer(helloworld_pb2_grpc.GreeterServicer):
    def __init__(self, transferType, XDTconfig=None):

        self.benchName = 'vhive-tuning'
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
            self.XDTconfig = XDTconfig

    def put(self, obj, key):
        msg = "Driver uploading object with key '" + key + "' to " + self.transferType
        log.info(msg)
        with tracing.Span(msg):
            pickled = pickle.dumps(obj)
            if self.transferType == S3:
                s3object = self.s3_client.Object(bucket_name=self.benchName, key=key)
                s3object.put(Body=pickled)
            elif self.transferType == XDT:
                log.fatal("XDT is not supported")

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
                log.fatal("XDT is not yet supported")

        return pickle.loads(response['Body'].read())

    def handler_broker(self, event, context):
        dataset = generate_dataset()
        hyperparam_config = {
            'model': 'RandomForestRegressor',
            'params': {
                'n_estimators': [5, 10, 20],
                'min_samples_split': [2, 4],
                'random_state': [42]
            }
        }
        models_config = {
            'models': [
                {
                    'model': 'RandomForestRegressor',
                    'params': hyperparam
                } for hyperparam in generate_hyperparam_sets(hyperparam_config['params'])
            ]
        }
        self.put(dataset, 'dataset_key')
        return {
            'dataset_key': 'dataset_key',
            'models_config': models_config
        }

    def train(self, arg: dict) -> dict:
        log.info("Invoke Trainer")
        channel = grpc.insecure_channel(args.tAddr)
        stub = tuning_pb2_grpc.TrainerStub(channel)

        resp = stub.Train(tuning_pb2.TrainRequest(
            dataset=b'',  # via S3/XDT only
            dataset_key="dataset_key",
            model_config=pickle.dumps(arg['model_config']),
            count=arg['count'],
            sample_rate=arg['sample_rate']
        ))

        return {
            'model_key': resp.model_key,
            'pred_key': resp.pred_key,
            'score': resp.score,
            'params': pickle.loads(resp.params),
        }

    # Driver code below
    def SayHello(self, request, context):
        log.info("Driver received a request")

        event = self.handler_broker({}, {})
        models = event['models_config']['models']

        while len(models)>1:
            sample_rate = 1/len(models)
            log.info(f"Running {len(models)} models on the dataset with sample rate {sample_rate} ")
            # Run different model configs on sampled dataset
            training_responses = []
            for count, model_config in enumerate(models):
                training_responses.append(
                    self.train({
                        'dataset_key': event['dataset_key'],
                        'model_config': model_config,
                        'count': count,
                        'sample_rate': sample_rate
                    })
                )

            # Keep models with the best score
            top_number = len(training_responses)//2
            sorted_responses = sorted(training_responses, key=lambda result: result['score'], reverse=True)
            models = [resp['params'] for resp in sorted_responses[:top_number]]

        log.info(f"Training final model {models[0]} on the full dataset")
        final_response = self.train({
            'dataset_key': event['dataset_key'],
            'model_config': models[0],
            'count': 0,
            'sample_rate': 1.0
        })

        log.info(f"Final result: score {final_response['score']}, model {final_response['params']['model']} ")
        return helloworld_pb2.HelloReply(message=self.benchName)


def serve():
    transferType = os.getenv('TRANSFER_TYPE', S3)
    if transferType == S3:
        log.info("Using inline or s3 transfers")
        max_workers = int(os.getenv("MAX_SERVER_THREADS", 10))
        server = grpc.server(futures.ThreadPoolExecutor(max_workers=max_workers))
        helloworld_pb2_grpc.add_GreeterServicer_to_server(
            GreeterServicer(transferType=transferType), server)
        SERVICE_NAMES = (
            helloworld_pb2.DESCRIPTOR.services_by_name['Greeter'].full_name,
            reflection.SERVICE_NAME,
        )
        reflection.enable_server_reflection(SERVICE_NAMES, server)
        server.add_insecure_port('[::]:' + args.sp)
        server.start()
        server.wait_for_termination()
    elif transferType == XDT:
        log.fatal("XDT not yet supported")
        XDTconfig = XDTutil.loadConfig()
    else:
        log.fatal("Invalid Transfer type")


if __name__ == '__main__':
    log.basicConfig(level=log.INFO)
    serve()
