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
import grpc
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

# adding python tracing sources to the system path
sys.path.insert(0, os.getcwd() + '/../proto/')
sys.path.insert(0, os.getcwd() + '/../../../../utils/tracing/python')
import tracing
import tuning_pb2_grpc
import tuning_pb2
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
    tracing.initTracer("trainer", url=args.zipkinURL)
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
	if model_name=='LinearSVR':
		return LinearSVR
	elif model_name=='LinearRegression':
		return LinearRegression
	elif model_name=='RandomForestRegressor':
		return RandomForestRegressor
	elif model_name=='KNeighborsRegressor':
		return KNeighborsRegressor
	elif model_name=='LogisticRegression':
		return LogisticRegression
	else:
		raise ValueError(f"Model {model_name} not found") 


class TrainerServicer(tuning_pb2_grpc.TrainerServicer):
    def __init__(self, transferType, XDTconfig=None):

        self.benchName = 'vhive-tuning'
        self.transferType = transferType
        self.trainer_id = ""
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
        msg = "Trainer uploading object with key '" + key + "' to " + self.transferType
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
        msg = "Trainer gets key '" + key + "' from " + self.transferType
        log.info(msg)
        with tracing.Span(msg):
            response = None
            if self.transferType == S3:
                obj = self.s3_client.Object(bucket_name=self.benchName, key=key)
                response = obj.get()
            elif self.transferType == XDT:
                log.fatal("XDT is not yet supported")

        return pickle.loads(response['Body'].read())

    def Train(self, request, context):
        # Read from S3
        dataset = self.get(request.dataset_key)

        with tracing.Span("Training a model"):
            model_config = pickle.loads(request.model_config)
            sample_rate = request.sample_rate
            count = request.count
            
            # Init model
            model_class = model_dispatcher(model_config['model'])
            model = model_class(**model_config['params'])

            # Train model and get predictions
            X = dataset['features']
            y = dataset['labels']
            if sample_rate==1.0:
                X_sampled, y_sampled = X, y
            else:
                stratified_split = StratifiedShuffleSplit(n_splits=1, train_size=sample_rate, random_state=42)
                sampled_index, _ = list(stratified_split.split(X, y))[0]
                X_sampled, y_sampled = X[sampled_index], y[sampled_index]
            y_pred = cross_val_predict(model, X_sampled, y_sampled, cv=5)
            model.fit(X_sampled, y_sampled)
            score = roc_auc_score(y_sampled, y_pred)
            log.info(f"{model_config['model']}, params: {model_config['params']}, dataset size: {len(y_sampled)},score: {score}")

        # Write to S3
        model_key = f"model_{count}"
        pred_key = f"pred_model_{count}"
        self.put(model, model_key)
        self.put(y_pred, pred_key)

        return tuning_pb2.TrainReply(
            model=b'',
            model_key=model_key,
            pred_key=pred_key,
            score=score,
            params=pickle.dumps(model_config),
        )


def serve():
    transferType = os.getenv('TRANSFER_TYPE', S3)
    if transferType == S3:
        log.info("Using inline or s3 transfers")
        max_workers = int(os.getenv("MAX_SERVER_THREADS", 10))
        server = grpc.server(futures.ThreadPoolExecutor(max_workers=max_workers))
        tuning_pb2_grpc.add_TrainerServicer_to_server(
            TrainerServicer(transferType=transferType), server)
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
