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

from concurrent import futures
from timeit import default_timer as now

start_overall = now()

parser = argparse.ArgumentParser()
parser.add_argument("-sp", "--sp", dest = "sp", default = "80", help="serve port")
parser.add_argument("-zipkin", "--zipkin", dest = "url", default = "http://zipkin.istio-system.svc.cluster.local:9411/api/v2/spans", help="Zipkin endpoint url")

args = parser.parse_args()

if tracing.IsTracingEnabled():
    tracing.initTracer("recog", url=args.url)
    tracing.grpcInstrumentServer()

# Load model
start_load_model = now()
model = models.squeezenet1_1(pretrained=True)
end_load_model = now()
load_model_e2e = (end_load_model - start_load_model) * 1000
print('Time to load model: %dms' % load_model_e2e)

labels_fd = open('imagenet_labels.txt', 'r')
labels = []
for i in labels_fd:
    labels.append(i)
labels_fd.close()

def processImage(bytes):
    start_preprocess = now()
    img = Image.open(io.BytesIO(bytes))

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
    batch_t = torch.unsqueeze(img_t, 0)

    end_preprocess = now()
    preprocess_e2e = (end_preprocess - start_preprocess) * 1000
    print('Time to preprocess: %dms' % preprocess_e2e)

    # Set up model to do evaluation
    model.eval()

    # Run inference
    start_inf = now()
    with torch.no_grad():
        out = model(batch_t)
    end_inf = now()
    inference_e2e = (end_inf - start_inf) * 1000
    print('Time to perform inference: %dms' % inference_e2e)

    # Print top 5 for logging
    _, indices = torch.sort(out, descending=True)
    percentages = torch.nn.functional.softmax(out, dim=1)[0] * 100
    for idx in indices[0][:5]:
        print('\tLabel: %s, percentage: %.2f' % (labels[idx], percentages[idx].item()))

    end_overall = now()
    total_e2e = (end_overall - start_overall) * 1000
    print('End-to-end time: %dms' % total_e2e)

    # Return top label and timers in binded output
    top_label = labels[indices[0][0]]
    output_msg = 'Label %s,LoadMod %d,PreProc %d,Inf %d,Tot %d' % (
        top_label, load_model_e2e, preprocess_e2e, inference_e2e, total_e2e)
    print(output_msg)

    # make comma-seperated output of top 100 label
    out = ""
    for idx in indices[0][:100]:
        out = out + labels[idx] + ","
    return out

class ObjectRecognitionServicer(videoservice_pb2_grpc.ObjectRecognitionServicer):
    def Recognise(self, request, context):
        print("received a call")
        with tracing.Span("Object recognition"):
            classification = processImage(request.frame)
        print("object recogintion successful")
        return videoservice_pb2.RecogniseReply(classification=classification)


def serve():
  server = grpc.server(futures.ThreadPoolExecutor(max_workers=10))
  videoservice_pb2_grpc.add_ObjectRecognitionServicer_to_server(
      ObjectRecognitionServicer(), server)
  server.add_insecure_port('[::]:'+args.sp)
  server.start()
  server.wait_for_termination()

if __name__ == '__main__':
  serve()
