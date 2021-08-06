# MIT License
#
# Copyright (c) 2021 EASE lab
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

# adding python tracing and storage sources to the system path
sys.path.insert(0, os.getcwd() + '/../proto/')
sys.path.insert(0, os.getcwd() + '/../../../../utils/tracing/python')
sys.path.insert(0, os.getcwd() + '/../../../../utils/storage/python')
import tracing
import storage
import helloworld_pb2_grpc
import helloworld_pb2
import stacking_pb2_grpc
import stacking_pb2
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
from sklearn.linear_model import LogisticRegression
from sklearn.neighbors import KNeighborsRegressor
from sklearn.ensemble import RandomForestRegressor
from sklearn.svm import LinearSVR
from sklearn.linear_model import LinearRegression, Lasso
from sklearn.model_selection import cross_val_predict
from sklearn.metrics import roc_auc_score
import numpy as np
import pickle

from concurrent import futures

parser = argparse.ArgumentParser()
parser.add_argument("-dockerCompose", "--dockerCompose", dest="dockerCompose", default=False, help="Env docker compose")
parser.add_argument("-tAddr", "--tAddr", dest="tAddr", default="trainer.default.192.168.1.240.sslip.io:80",
                    help="trainer address")
parser.add_argument("-rAddr", "--rAddr", dest="rAddr", default="reducer.default.192.168.1.240.sslip.io:80",
                    help="reducer address")
parser.add_argument("-mAddr", "--mAddr", dest="mAddr", default="metatrainer.default.192.168.1.240.sslip.io:80",
                    help="metatrainer address")
parser.add_argument("-trainersNum", "--trainersNum", dest="trainersNum", default="3", help="number of training models")
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

model_config = {
    'models': [
        {
            'model': 'LinearSVR',
            'params': {
                'C': 1.0,
                'tol': 1e-6,
                'random_state': 42
            }
        },
        {
            'model': 'Lasso',
            'params': {
                'alpha': 0.1
            }
        },
        {
            'model': 'RandomForestRegressor',
            'params': {
                'n_estimators': 2,
                'max_depth': 2,
                'min_samples_split': 2,
                'min_samples_leaf': 2,
                # 'n_jobs': 2,
                'random_state': 42
            }
        },
        {
            'model': 'KNeighborsRegressor',
            'params': {
                'n_neighbors': 20,
            }
        }
    ],
    'meta_model': {
        'model': 'LogisticRegression',
        'params': {}
    }
}


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
    n_samples = 300
    n_features = 1024
    X, y = datasets.make_classification(n_samples,
                                        n_features,
                                        n_redundant=0,
                                        n_clusters_per_class=2,
                                        weights=[0.9, 0.1],
                                        flip_y=0.1,
                                        random_state=42)
    return {'features': X, 'labels': y}


def reduce(training_responses) -> dict:
    log.info("Invoke Reducer")
    channel = grpc.insecure_channel(args.rAddr)
    stub = stacking_pb2_grpc.ReducerStub(channel)

    model_keys = []
    pred_keys = []

    req = stacking_pb2.ReduceRequest()
    for resp in training_responses:
        model_pred_tuple = stacking_pb2.ModelPredTuple()
        model_pred_tuple.model_key = resp['model_key']
        model_pred_tuple.pred_key = resp['pred_key']

        req.model_pred_tuples.append(model_pred_tuple)

    resp = stub.Reduce(req)

    return {
        'meta_features_key': resp.meta_features_key,
        'models_key': resp.models_key
    }


class GreeterServicer(helloworld_pb2_grpc.GreeterServicer):
    def __init__(self, transferType, XDTconfig=None):

        self.benchName = 'vhive-stacking'
        self.dataset = generate_dataset()
        self.modelConfig = model_config
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

    def train(self, arg: dict) -> dict:
        log.info(f"Invoke Trainer {arg['trainer_id']}")
        channel = grpc.insecure_channel(args.tAddr)
        stub = stacking_pb2_grpc.TrainerStub(channel)

        resp = stub.Train(stacking_pb2.TrainRequest(
            dataset=b'',  # via S3/XDT only
            dataset_key=arg['dataset_key'],
            model_config=pickle.dumps(arg['model_cfg']),
            trainer_id=str(arg['trainer_id'])
        ))

        return {
            'model_key': resp.model_key,
            'pred_key': resp.pred_key
        }

    def train_all(self, dataset_key: str) -> list:
        log.info("Invoke Trainers")

        with tracing.Span("Invoke all trainers"):
            all_result_futures = []
            # send all requests
            trainers_num: int = int(os.getenv('TrainersNum', args.trainersNum))
            models = self.modelConfig['models']
            training_responses = []

            if os.getenv('CONCURRENT_TRAINING', "false").lower() == "false":
                for i in range(trainers_num):
                    all_result_futures.append(
                        self.train(
                            {
                                'dataset_key': dataset_key,
                                'model_cfg': models[i % len(models)],
                                'trainer_id': i
                            })
                    )
            else:
                ex = futures.ThreadPoolExecutor(max_workers=trainers_num)
                all_result_futures = ex.map(
                    self.train,
                    [{
                        'dataset_key': dataset_key,
                        'model_cfg': models[i % len(models)],
                        'trainer_id': i
                    } for i in range(trainers_num)]
                )
            log.info("Retrieving trained models")
            for result in all_result_futures:
                training_responses.append(result)

        return training_responses

    def train_meta(self, dataset_key: str, reducer_response: dict) -> dict:
        log.info("Invoke MetaTrainer")
        channel = grpc.insecure_channel(args.mAddr)
        stub = stacking_pb2_grpc.MetatrainerStub(channel)

        resp = stub.Metatrain(stacking_pb2.MetaTrainRequest(
            dataset=b'',  # via S3/XDT only
            dataset_key=dataset_key,
            models_key=reducer_response['models_key'],  # via S3 only
            meta_features=b'',
            meta_features_key=reducer_response['meta_features_key'],
            model_config=pickle.dumps(self.modelConfig['meta_model'])
        ))

        return {
            'model_full_key': resp.model_full_key,
            'meta_predictions_key': resp.meta_predictions_key,
            'score': resp.score
        }

    def get_final(self, outputs: dict):
        log.info("Get the final outputs")

        _ = storage.get(outputs['model_full_key'])
        _ = storage.get(outputs['meta_predictions_key'])

    # Driver code below
    def SayHello(self, request, context):
        log.info("Driver received a request")
        
        dataset_key = storage.put("dataset", self.dataset)

        training_responses = self.train_all(dataset_key)

        reducer_response = reduce(training_responses)

        outputs = self.train_meta(dataset_key, reducer_response)

        self.get_final(outputs)

        return helloworld_pb2.HelloReply(message=self.benchName)


def serve():
    transferType = os.getenv('TRANSFER_TYPE', S3)
    if transferType == S3:
        storage.init("S3", 'vhive-stacking')
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
