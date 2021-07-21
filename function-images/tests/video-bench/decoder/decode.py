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
import videoservice_pb2_grpc
import videoservice_pb2

import cv2
import grpc
import tempfile
import argparse
import boto3
import logging as log

from concurrent import futures
from multiprocessing.pool import ThreadPool as Pool
import multiprocessing
from itertools import repeat

# USE ENV VAR "DecoderFrames" to set the number of frames to be sent
parser = argparse.ArgumentParser()
parser.add_argument("-addr", "--addr", dest = "addr", default = "recog.default.192.168.1.240.sslip.io:80", help="recog address")
parser.add_argument("-sp", "--sp", dest = "sp", default = "80", help="serve port")
parser.add_argument("-frames", "--frames", dest = "frames", default = "1", help="Default number of frames- overwritten by environment variable")
parser.add_argument("-zipkin", "--zipkin", dest = "url", default = "http://zipkin.istio-system.svc.cluster.local:9411/api/v2/spans", help="Zipkin endpoint url")

args = parser.parse_args()

if tracing.IsTracingEnabled():
    tracing.initTracer("decoder", url=args.url)
    tracing.grpcInstrumentClient()
    tracing.grpcInstrumentServer()


# set aws credentials:
AWS_ID = os.getenv('AWS_ACCESS_KEY', "")
AWS_SECRET = os.getenv('AWS_SECRET_KEY', "")
s3_client = boto3.resource(
    service_name='s3',
    region_name=os.getenv("AWS_REGION", 'us-west-1'),
    aws_access_key_id=AWS_ID,
    aws_secret_access_key=AWS_SECRET
)


def decode(bytes):
    temp = tempfile.NamedTemporaryFile(suffix=".mp4")
    temp.write(bytes)
    temp.seek(0)

    all_frames = [] 
    with tracing.Span("Decode frames"):
        vidcap = cv2.VideoCapture(temp.name)
        for i in range(int(os.getenv('DecoderFrames', int(args.frames)))):
            success,image = vidcap.read()
            all_frames.append(cv2.imencode('.jpg', image)[1].tobytes())

    return all_frames


def fetchFromS3(key):
    obj = s3_client.Object(bucket_name='vhive-video-bench', key=key)
    response = obj.get()
    return response['Body'].read()


def putToS3(key, obj):
    s3object = s3_client.Object('vhive-video-bench', key)
    log.info("uploading %s to s3" % key)
    s3object.put(Body=obj)


class VideoDecoderServicer(videoservice_pb2_grpc.VideoDecoderServicer):
    def __init__(self):
        uses3 = os.getenv('USES3', "false")
        if uses3 == "false":
            self.uses3 = False
        elif uses3 == "true":
            self.uses3 = True
        else:
            log.info("Invalid USES3 value")


    def Decode(self, request, context):
        log.info("Decoder recieved a request")

        out = []
        if self.uses3:
            log.info("Using s3, getting bucket")
            with tracing.Span("Video fetch"):
                data = fetchFromS3(request.s3key)
            log.info("decoding frames of the s3 object")
            out = decode(data)
        else:
            log.info("Standard video decode (no s3). Decoding frames.")
            out = decode(request.video)

        with tracing.Span("Recognise all frames"):
            # all_result_futures = []
            # send all requests
            out = out[0:int(os.getenv('DecoderFrames', int(args.frames)))]
            frame_num = range(len(out))
            pool = multiprocessing.Pool()
            all_result_futures = pool.starmap(recognise,  zip(repeat(self.uses3), out, frame_num))
            # for frame in out:
            #     all_result_futures.append(self.Recognise(frame))
            # concat all results
            log.info("returning result of frame classification")
            results = ""
            for result in all_result_futures:
                results = results + result + ","

            return videoservice_pb2.DecodeReply(classification=results)


def recognise(uses3, frame, frame_num):
    tracing.initTracer("decoder-thread", url=args.url)
    channel = grpc.insecure_channel(args.addr)
    stub = videoservice_pb2_grpc.ObjectRecognitionStub(channel)
    if uses3:
        name = "decoder-frame-" + str(frame_num) + ".jpg"
        with tracing.Span("Upload frame"):
            putToS3(name, frame)
        log.info("calling recog with s3 key")
        response_future = stub.Recognise.future(videoservice_pb2.RecogniseRequest(s3key=name))
    else:
        response_future = stub.Recognise.future(videoservice_pb2.RecogniseRequest(frame=frame))

    return response_future.result().classification


def serve():
    max_workers = int(os.getenv("MAX_DECODER_SERVER_THREADS", 10))
    server = grpc.server(futures.ThreadPoolExecutor(max_workers=max_workers))
    videoservice_pb2_grpc.add_VideoDecoderServicer_to_server(
        VideoDecoderServicer(), server)
    server.add_insecure_port('[::]:'+args.sp)
    server.start()
    server.wait_for_termination()


if __name__ == '__main__':
    log.basicConfig(level=log.INFO)
    serve()
