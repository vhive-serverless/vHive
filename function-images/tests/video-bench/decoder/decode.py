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
import boto3


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


# set aws credentials:
AWS_ID = os.getenv('AWS_ACCESS_KEY', "")
AWS_SECRET = os.getenv('AWS_SECRET_KEY', "")

def decode(bytes):
    start_tempfile = now()
    temp = tempfile.NamedTemporaryFile(suffix=".mp4")
    temp.write(bytes)
    temp.seek(0)
    end_tempfile_write = now()
    write_time = (end_tempfile_write - start_tempfile)*1000
    print("tempfile write time: %dms" %write_time)

    all_frames = [] 
    with tracing.Span("Decode frames"):
        vidcap = cv2.VideoCapture(temp.name)
        for i in range(int(os.getenv('DecoderFrames', int(args.frames)))):
            success,image = vidcap.read()
            all_frames.append(cv2.imencode('.jpg', image)[1].tobytes())

    return all_frames



class VideoDecoderServicer(videoservice_pb2_grpc.VideoDecoderServicer):
    def Decode(self, request, context):
        print("Decoder recieved a request")
        uses3 = os.getenv('USES3', "false")
        if uses3 == "false":
            uses3 = False
        elif uses3 == "true":
            uses3 = True
        else:
            print("Invalid USES3 value")

        out = []
        s3 = None
        if uses3:
            print("Using s3, getting bucket")
            s3 = boto3.resource(
                service_name='s3',
                region_name='us-west-1',
                aws_access_key_id=AWS_ID,
                aws_secret_access_key= AWS_SECRET
            )
            obj = s3.Object(bucket_name='vhive-video-bench', key=request.s3key)
            response = obj.get()
            data = response['Body'].read()
            print("decoding frames of the s3 object")
            out = decode(data)
        else:
            print("Standard video decode (no s3). Decoding frames.")
            out = decode(request.video)

        with tracing.Span("Recognise all frames"):  
            all_result_futures = []
            # send all requests
            self.frameCount = 0
            for i in range(int(os.getenv('DecoderFrames', int(args.frames)))):
                all_result_futures.append(self.Recognise(out[i], s3))
            # concat all results
            print("returning result of frame classification")
            results = ""
            for result in all_result_futures:
                results = results + result.result().classification + ","

            return videoservice_pb2.DecodeReply(classification=results)
    
    def Recognise(self, frame, s3):
        channel = grpc.insecure_channel(args.addr)
        stub = videoservice_pb2_grpc.ObjectRecognitionStub(channel)
        if (s3 != None):
            name = "decoder-frame-" + str(self.frameCount) + ".jpg"
            self.frameCount += 1
            s3object = s3.Object('vhive-video-bench', name)
            print("uploading frame %d to s3" % (self.frameCount-1))
            s3object.put(Body=frame)
            print("calling recog with s3 key")
            response_future = stub.Recognise.future(videoservice_pb2.RecogniseRequest(s3key=name))
        else:
            response_future = stub.Recognise.future(videoservice_pb2.RecogniseRequest(frame=frame))
        
        return response_future



def serve():
    server = grpc.server(futures.ThreadPoolExecutor(max_workers=10))
    videoservice_pb2_grpc.add_VideoDecoderServicer_to_server(
        VideoDecoderServicer(), server)
    server.add_insecure_port('[::]:'+args.sp)
    server.start()
    server.wait_for_termination()



if __name__ == '__main__':
    serve()
