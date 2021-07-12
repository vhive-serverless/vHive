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

from concurrent import futures
from timeit import default_timer as now

# USE ENV VAR "DecoderFrames" to set the number of frames to be sent
parser = argparse.ArgumentParser()
parser.add_argument("-addr", "--addr", dest = "addr", default = "recog.default.192.168.1.240.sslip.io:80", help="recog address")
parser.add_argument("-sp", "--sp", dest = "sp", default = "80", help="serve port")
parser.add_argument("-zipkin", "--zipkin", dest = "url", default = "http://zipkin.istio-system.svc.cluster.local:9411/api/v2/spans", help="Zipkin endpoint url")

args = parser.parse_args()

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

  with tracing.Span("Split into frames"):  
    vidcap = cv2.VideoCapture(temp.name)
    

    start_decode = now()

    success,image = vidcap.read()
    count = 0

    while success and count < 10:
      cv2.imwrite("frames/frame%d.jpg" % count, image)     # save frame as JPEG file      
      success,image = vidcap.read()
      count += 1

    end_decode = now()

    decode_e2e = ( end_decode - start_decode ) * 1000
    print('Time to decode %d frames: %dms' % (count, decode_e2e))
    temp.close()

def SendFrame(frameNumber):
  frame = open("frames/frame%d.jpg" % frameNumber, "rb")
  channel = grpc.insecure_channel(args.addr)
  stub = videoservice_pb2_grpc.ProcessFrameStub(channel)
  response = stub.SendFrame(videoservice_pb2.SendFrameRequest(value=frame.read()))
  return response.value


class DecodeVideoServicer(videoservice_pb2_grpc.DecodeVideoServicer):
  def SendVideo(self, request, context):
    decode(request.value)
    with tracing.Span("Send all frames"):  
      for i in range(os.getenv('DecoderFrames', 1)):
        result = SendFrame(i)
        print(result)
    # just returns the final result
    return videoservice_pb2.SendVideoReply(value=result)


def serve():
  server = grpc.server(futures.ThreadPoolExecutor(max_workers=10))
  videoservice_pb2_grpc.add_DecodeVideoServicer_to_server(
      DecodeVideoServicer(), server)
  server.add_insecure_port('[::]:'+args.sp)
  server.start()
  server.wait_for_termination()



if __name__ == '__main__':
  serve()
