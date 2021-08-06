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
# adding python tracing and storage sources to the system path
sys.path.insert(0, os.getcwd() + '/../proto/')
sys.path.insert(0, os.getcwd() + '/../../../../utils/tracing/python')
sys.path.insert(0, os.getcwd() + '/../../../../utils/storage/python')
import tracing
import storage
import videoservice_pb2_grpc
import videoservice_pb2
import destination as XDTdst
import source as XDTsrc
import utils as XDTutil

import cv2
import grpc
import tempfile
import argparse
import boto3
import logging as log
import socket

from concurrent import futures

# USE ENV VAR "DecoderFrames" to set the number of frames to be sent
parser = argparse.ArgumentParser()
parser.add_argument("-dockerCompose", "--dockerCompose", dest="dockerCompose", default=False, help="Env docker compose")
parser.add_argument("-addr", "--addr", dest="addr", default="recog.default.svc.cluster.local:80", help="recog address")
parser.add_argument("-sp", "--sp", dest="sp", default="80", help="serve port")
parser.add_argument("-frames", "--frames", dest="frames", default="1", help="Default number of frames- overwritten by environment variable")
parser.add_argument("-zipkin", "--zipkin", dest="url", default="http://zipkin.istio-system.svc.cluster.local:9411/api/v2/spans", help="Zipkin endpoint url")

args = parser.parse_args()

if tracing.IsTracingEnabled():
    tracing.initTracer("decoder", url=args.url)
    tracing.grpcInstrumentClient()
    tracing.grpcInstrumentServer()

INLINE = "INLINE"
S3 = "S3"
XDT = "XDT"

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


class VideoDecoderServicer(videoservice_pb2_grpc.VideoDecoderServicer):
    def __init__(self, transferType, XDTconfig=None):

        self.frameCount = 0
        self.transferType = transferType
        if transferType == XDT:
            if XDTconfig is None:
                log.fatal("Empty XDT config")
            self.XDTconfig = XDTconfig

    def Decode(self, request, context):
        log.info("Decoder recieved a request")

        videoBytes = b''
        if self.transferType == S3:
            log.info("Using s3, getting bucket")
            with tracing.Span("Video fetch"):
                videoBytes = storage.get(request.s3key, dontPickle=True)
            log.info("decoding frames of the s3 object")
        elif self.transferType == INLINE:
            log.info("Inline video decode. Decoding frames.")
            videoBytes = request.video
        results = self.processFrames(videoBytes)
        return videoservice_pb2.DecodeReply(classification=results)

    def processFrames(self, videoBytes):
        frames = decode(videoBytes)
        with tracing.Span("Recognise all frames"):  
            all_result_futures = []
            # send all requests
            decoderFrames = int(os.getenv('DecoderFrames', args.frames))
            frames = frames[0:decoderFrames]
            if os.getenv('CONCURRENT_RECOG', "false").lower() == "false":
                # concat all results
                for frame in frames:
                    all_result_futures.append(self.Recognise(frame))
            else:
                ex = futures.ThreadPoolExecutor(max_workers=decoderFrames)
                all_result_futures = ex.map(self.Recognise, frames)
            log.info("returning result of frame classification")
            results = ""
            for result in all_result_futures:
                results = results + result + ","

            return results
    
    def Recognise(self, frame):
        channel = grpc.insecure_channel(args.addr)
        stub = videoservice_pb2_grpc.ObjectRecognitionStub(channel)
        result = b''
        if self.transferType == S3:
            name = "decoder-frame-" + str(self.frameCount) + ".jpg"
            with tracing.Span("Upload frame"):
                self.frameCount += 1
                storage.put(name, frame)
            log.info("calling recog with s3 key")
            response = stub.Recognise(videoservice_pb2.RecogniseRequest(s3key=name))
            result = response.classification
        elif self.transferType == INLINE:
            response = stub.Recognise(videoservice_pb2.RecogniseRequest(frame=frame))
            result = response.classification
        elif self.transferType == XDT:
            xdtPayload = XDTutil.Payload(FunctionName="HelloXDT", Data=frame)
            if not args.dockerCompose:
                log.info("replacing SQP hostname")
                self.XDTconfig["SQPServerHostname"] = get_self_ip()
            response_bytes, ok = XDTsrc.InvokeWithXDT(args.addr, xdtPayload, self.XDTconfig)
            # convert response bytes to string
            result = response_bytes.decode()
        
        return result


def serve():
    transferType = os.getenv('TRANSFER_TYPE', INLINE)
    if transferType == S3:
        storage.init("S3", 'vhive-video-bench')
    if transferType == S3 or transferType == INLINE:
        max_workers = int(os.getenv("MAX_DECODER_SERVER_THREADS", 10))
        server = grpc.server(futures.ThreadPoolExecutor(max_workers=max_workers))
        videoservice_pb2_grpc.add_VideoDecoderServicer_to_server(
            VideoDecoderServicer(transferType=transferType), server)
        server.add_insecure_port('[::]:'+args.sp)
        server.start()
        server.wait_for_termination()
    elif transferType == XDT:
        XDTconfig = XDTutil.loadConfig()
        log.info("[decode] transfering via XDT")
        log.info(XDTconfig)

        def handler(videoBytes):
            decoderService = VideoDecoderServicer(transferType=transferType, XDTconfig=XDTconfig)
            results = decoderService.processFrames(videoBytes)
            return results.encode(), True

        XDTdst.StartDstServer(XDTconfig, handler)
    else:
        log.fatal("Invalid Transfer type")


if __name__ == '__main__':
    log.basicConfig(level=log.INFO)
    serve()
