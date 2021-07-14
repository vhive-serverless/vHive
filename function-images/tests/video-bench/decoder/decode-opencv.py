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

import io
import cv2
import grpc
import tempfile
import argparse
import ffmpeg

from concurrent import futures
from timeit import default_timer as now

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

def decode(bytes):
    with tracing.Span("Making Tempfile"):
        start_tempfile = now()
        temp = tempfile.NamedTemporaryFile(suffix=".mp4")
        temp.write(bytes)
        temp.seek(0)
        end_tempfile_write = now()
        write_time = (start_tempfile-end_tempfile_write)*1000
        print("tempfile write time: %dms" %write_time)

    all_frames = [] 
    with tracing.Span("Get frames"):
        vidcap = cv2.VideoCapture(temp.name)
        for i in range(os.getenv('DecoderFrames', int(args.frames))):
            success,image = vidcap.read()
            all_frames.append(cv2.imencode('.jpg', image)[1].tobytes())

    return all_frames



class DecodeVideoServicer(videoservice_pb2_grpc.DecodeVideoServicer):
    def SendVideo(self, request, context):
        out = decode(request.value)
        with tracing.Span("Send all frames"):  
            for i in range(os.getenv('DecoderFrames', int(args.frames))):
                print("SENDING FRAME %d" %i)
                result = self.SendFrame(out[i])
                print(result)
        # just returns the final result
        return videoservice_pb2.SendVideoReply(value=result)
    
    def SendFrame(self, frame):
        channel = grpc.insecure_channel(args.addr)
        stub = videoservice_pb2_grpc.ProcessFrameStub(channel)
        response = stub.SendFrame(videoservice_pb2.SendFrameRequest(value=frame))
        return response.value



def serve():
    server = grpc.server(futures.ThreadPoolExecutor(max_workers=10))
    videoservice_pb2_grpc.add_DecodeVideoServicer_to_server(
        DecodeVideoServicer(), server)
    server.add_insecure_port('[::]:'+args.sp)
    server.start()
    server.wait_for_termination()



if __name__ == '__main__':
    serve()
