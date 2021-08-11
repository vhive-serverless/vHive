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

import helloworld_pb2_grpc
import helloworld_pb2

import source as XDTsrc
import utils as XDTutil

from grpc_reflection.v1alpha import reflection
import grpc

import argparse

import logging as log


from concurrent import futures

# USE ENV VAR "DecoderFrames" to set the number of frames to be sent
parser = argparse.ArgumentParser()
parser.add_argument("-dockerCompose", "--dockerCompose", dest="dockerCompose", default=False, help="Env docker compose")
parser.add_argument("-addr", "--addr", dest="addr", default="recog.default.svc.cluster.local:80", help="recog address")
parser.add_argument("-sp", "--sp", dest="sp", default="80", help="serve port")
parser.add_argument("-frames", "--frames", dest="frames", default="1", help="Default number of frames- overwritten by environment variable")
parser.add_argument("-zipkin", "--zipkin", dest="url", default="http://zipkin.istio-system.svc.cluster.local:9411/api/v2/spans", help="Zipkin endpoint url")

args = parser.parse_args()


class GreeterServicer(helloworld_pb2_grpc.GreeterServicer):
    def __init__(self, consumerAddr="consumer.default.192.168.1.240.sslip.io:80", XDTconfig=None):

        self.consumerAddr = consumerAddr
        if XDTconfig is None:
            log.fatal("Empty XDT config")
        self.XDTclient = XDTsrc.XDTclient(XDTconfig)
        self.XDTconfig = XDTconfig

    # Driver code below
    def SayHello(self, request, context):
        data = bytes(os.urandom(1024 * 1024 * 10))
        payload = XDTutil.Payload(FunctionName="foo", Data=data)
        message, ok = self.xdtClient.Invoke(URL=self.consumerAddr, xdtPayload=payload)
        log.info("received %d and %d from invoke", message, ok)
        return helloworld_pb2.HelloReply(message=self.benchName)


def serve():

    XDTconfig = XDTutil.loadConfig()
    log.info("[decode] transfering via XDT")
    if not args.dockerCompose:
        log.info("replacing SQP hostname")
        XDTconfig["SQPServerHostname"] = XDTutil.get_self_ip()
    log.info(XDTconfig)

    max_workers = int(os.getenv("MAX_DECODER_SERVER_THREADS", 10))
    server = grpc.server(futures.ThreadPoolExecutor(max_workers=max_workers))
    helloworld_pb2_grpc.add_GreeterServicer_to_server(
        GreeterServicer(XDTconfig=XDTconfig), server)
    SERVICE_NAMES = (
        helloworld_pb2.DESCRIPTOR.services_by_name['Greeter'].full_name,
        reflection.SERVICE_NAME,
    )
    reflection.enable_server_reflection(SERVICE_NAMES, server)
    server.add_insecure_port('[::]:'+args.sp)
    server.start()
    server.wait_for_termination()


if __name__ == '__main__':
    log.basicConfig(level=log.INFO)
    serve()
