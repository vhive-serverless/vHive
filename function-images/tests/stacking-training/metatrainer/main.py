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
import grpc
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

# adding python tracing sources to the system path
sys.path.insert(0, os.getcwd() + '/../proto/')
sys.path.insert(0, os.getcwd() + '/../../../../utils/tracing/python')
import tracing
import stacking_pb2_grpc
import stacking_pb2
import destination as XDTdst
import source as XDTsrc
import utils as XDTutil



from concurrent import futures

parser = argparse.ArgumentParser()
parser.add_argument("-dockerCompose", "--dockerCompose", dest="dockerCompose", default=False, help="Env docker compose")
parser.add_argument("-sp", "--sp", dest="sp", default="80", help="serve port")
parser.add_argument("-zipkin", "--zipkin", dest="zipkinURL",
                    default="http://zipkin.istio-system.svc.cluster.local:9411/api/v2/spans",
                    help="Zipkin endpoint url")

args = parser.parse_args()

if tracing.IsTracingEnabled():
    tracing.initTracer("metatrainer", url=args.zipkinURL)
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


def model_dispatcher(model_name):
    if model_name == 'LinearSVR':
        return LinearSVR
    elif model_name == 'Lasso':
        return Lasso
    elif model_name == 'LinearRegression':
        return LinearRegression
    elif model_name == 'RandomForestRegressor':
        return RandomForestRegressor
    elif model_name == 'KNeighborsRegressor':
        return KNeighborsRegressor
    elif model_name == 'LogisticRegression':
        return LogisticRegression
    else:
        raise ValueError(f"Model {model_name} not found")


class MetatrainerServicer(stacking_pb2_grpc.TrainerServicer):
    def __init__(self, transferType, XDTconfig=None):

        self.benchName = 'vhive-stacking'
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

    def put(self, obj, key):
        msg = "Driver uploading object with key '" + key + "' to " + self.transferType
        log.info(msg)
        with tracing.Span(msg):
            pickled = pickle.dumps(obj)
            if self.transferType == S3:
                s3object = self.s3_client.Object(bucket_name=self.benchName, key=key)
                s3object.put(Body=pickled)
            elif self.transferType == XDT:
                key = self.XDTclient.BroadcastPut(payload=pickled)

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
                return pickle.loads(XDTdst.BroadcastGet(key, self.XDTconfig))

        return pickle.loads(response['Body'].read())

    def get_inputs(self, request) -> dict:
        inputs: dict = {}
        inputs['dataset'] = self.get(request.dataset_key)
        inputs['meta_features'] = self.get(request.meta_features_key)
        inputs['models'] = self.get(request.models_key)
        return inputs

    def put_outputs(self, meta_predictions, model_full):
        model_full_key = 'model_full_key'
        meta_predictions_key = 'meta_predictions_key'

        meta_predictions_key = self.put(meta_predictions, meta_predictions_key)
        model_full_key = self.put(model_full, model_full_key)

        return meta_predictions_key, model_full_key

    def Metatrain(self, request, context):
        log.info(f"Metatrainer is invoked")

        with tracing.Span("Get the inputs"):
            inputs = self.get_inputs(request)
            dataset = inputs['dataset']
            meta_features = inputs['meta_features']
            models = inputs['models']

        log.info("Init meta model")
        model_config = pickle.loads(request.model_config)
        model_class = model_dispatcher(model_config['model'])
        meta_model = model_class(*model_config['params'])

        with tracing.Span("Train the meta model"):
            log.info("Train meta model and get predictions")
            meta_predictions = cross_val_predict(meta_model, meta_features, dataset['labels'], cv=5)
            score = roc_auc_score(meta_predictions, dataset['labels'])
            log.info(f"Ensemble model score {score}")
            meta_model.fit(meta_features, dataset['labels'])
            model_full = {
                'models': models,
                'meta_model': meta_model
            }

        with tracing.Span("Put the full model and predictions"):
            meta_predictions_key, model_full_key = self.put_outputs(meta_predictions, model_full)

        return stacking_pb2.MetaTrainReply(
            model_full=b'',
            model_full_key=model_full_key,
            meta_predictions=b'',
            meta_predictions_key=meta_predictions_key,
            score=str(score)
        )


def serve():
    transferType = os.getenv('TRANSFER_TYPE', S3)

    XDTconfig = dict()
    if transferType == XDT:
        XDTconfig = XDTutil.loadConfig()
        log.info("XDT config:")
        log.info(XDTconfig)

    log.info("Using %s transfers",transferType)
    max_workers = int(os.getenv("MAX_SERVER_THREADS", 10))
    server = grpc.server(futures.ThreadPoolExecutor(max_workers=max_workers))
    stacking_pb2_grpc.add_MetatrainerServicer_to_server(
        MetatrainerServicer(transferType=transferType, XDTconfig=XDTconfig), server)
    server.add_insecure_port('[::]:' + args.sp)
    server.start()
    server.wait_for_termination()


if __name__ == '__main__':
    log.basicConfig(level=log.INFO)
    serve()
