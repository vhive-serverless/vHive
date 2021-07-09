import sys
import os
# adding python tracing sources to the system path
sys.path.insert(0, os.getcwd() + '/../proto/')
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

args = parser.parse_args()


def decode(bytes):
  temp = tempfile.NamedTemporaryFile(suffix=".mp4")
  temp.write(bytes)
  temp.seek(0)
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
