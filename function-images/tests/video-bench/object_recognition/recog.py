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

"""Azure Function to perform inference.
"""
from torchvision import transforms
from PIL import Image
import torch
import torchvision.models as models

import sys
import os
# adding python tracing sources to the system path
sys.path.insert(0, os.getcwd() + '/../proto/')
sys.path.insert(0, os.getcwd() + '/../../../../utils/tracing/python')
import tracing
import videoservice_pb2_grpc
import videoservice_pb2

import io
import grpc
import argparse
import boto3
import logging as log

from concurrent import futures
from timeit import default_timer as now


parser = argparse.ArgumentParser()
parser.add_argument("-sp", "--sp", dest = "sp", default = "80", help="serve port")
parser.add_argument("-zipkin", "--zipkin", dest = "url", default = "http://zipkin.istio-system.svc.cluster.local:9411/api/v2/spans", help="Zipkin endpoint url")

args = parser.parse_args()

# set aws credentials:
AWS_ID = os.getenv('AWS_ACCESS_KEY', "")
AWS_SECRET = os.getenv('AWS_SECRET_KEY', "")


if tracing.IsTracingEnabled():
    tracing.initTracer("recog", url=args.url)
    tracing.grpcInstrumentServer()

# Load model
model = models.squeezenet1_1(pretrained=True)

labels_fd = open('imagenet_labels.txt', 'r')
labels = []
for i in labels_fd:
    labels.append(i)
labels_fd.close()


def preProcessImage(imageBytes):
    with tracing.Span("preprocess"):
        img = Image.open(io.BytesIO(imageBytes))

        transform = transforms.Compose([
            transforms.Resize(256),
            transforms.CenterCrop(224),
            transforms.ToTensor(),
            transforms.Normalize(
                mean=[0.485, 0.456, 0.406],
                std=[0.229, 0.224, 0.225]
            )
        ])

        img_t = transform(img)
        return torch.unsqueeze(img_t, 0)


def infer(batch_t):
    with tracing.Span("infer"):
        # Set up model to do evaluation
        model.eval()

        # Run inference
        with torch.no_grad():
            out = model(batch_t)

        # Print top 5 for logging
        _, indices = torch.sort(out, descending=True)
        percentages = torch.nn.functional.softmax(out, dim=1)[0] * 100
        for idx in indices[0][:5]:
            log.info('\tLabel: %s, percentage: %.2f' % (labels[idx], percentages[idx].item()))

        # make comma-seperated output of top 100 label
        out = ""
        for idx in indices[0][:100]:
            out = out + labels[idx] + ","
        return out


def fetchFrameS3(key):
    s3_client = boto3.resource(
        service_name='s3',
        region_name=os.getenv("AWS_REGION", 'us-west-1'),
        aws_access_key_id=AWS_ID,
        aws_secret_access_key=AWS_SECRET
    )
    obj = s3_client.Object(bucket_name='vhive-video-bench', key=key)
    response = obj.get()
    return response['Body'].read()


class ObjectRecognitionServicer(videoservice_pb2_grpc.ObjectRecognitionServicer):
    def Recognise(self, request, context):
        log.info("received a call")
        uses3 = os.getenv('USES3', "false")
        if uses3 == "false":
            uses3 = False
        elif uses3 == "true":
            uses3 = True
        else:
            log.info("Invalid USES3 value")

        # get the frame from s3 or inline
        frame = None
        if uses3:
            log.info("retrieving target frame '%s' from s3" % request.s3key)
            with tracing.Span("Frame fetch"):
                frame = fetchFrameS3(request.s3key)
        else:
            frame = request.frame

        log.info("performing image recognition on frame")
        classification = infer(preProcessImage(frame))
        log.info("object recogintion successful")
        return videoservice_pb2.RecogniseReply(classification=classification)


def serve():
    max_workers = int(os.getenv("MAX_RECOG_SERVER_THREADS", 10))
    server = grpc.server(futures.ThreadPoolExecutor(max_workers=max_workers))
    videoservice_pb2_grpc.add_ObjectRecognitionServicer_to_server(
        ObjectRecognitionServicer(), server)
    server.add_insecure_port('[::]:'+args.sp)
    server.start()
    server.wait_for_termination()


if __name__ == '__main__':
    log.basicConfig(level=log.INFO)
    serve()
