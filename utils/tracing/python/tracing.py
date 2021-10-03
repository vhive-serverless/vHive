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

import contextlib
import os

from opentelemetry.sdk.resources import SERVICE_NAME, Resource

from opentelemetry import trace
from opentelemetry.sdk.trace import TracerProvider
from opentelemetry.exporter.zipkin.json import ZipkinExporter
from opentelemetry.sdk.trace.export import BatchSpanProcessor
from opentelemetry.instrumentation.grpc import GrpcInstrumentorClient, GrpcInstrumentorServer
from opentelemetry import trace
from opentelemetry.sdk.trace.export import (
    ConsoleSpanExporter,
    SimpleSpanProcessor,
)

def IsTracingEnabled():
    val = os.getenv('ENABLE_TRACING', "false")
    print("ISTRACINGENABLED: %s" % val)
    if val == "false":
        return False
    else:
        return True

def initTracer(name, debug=False, url="http://localhost:9411/api/v2/spans"):
    trace.set_tracer_provider(TracerProvider(resource=Resource.create({SERVICE_NAME: name})))

    zipkin_exporter = ZipkinExporter(endpoint=url)
    span_processor = BatchSpanProcessor(zipkin_exporter)
    trace.get_tracer_provider().add_span_processor(span_processor)
    if (debug):
        trace.get_tracer_provider().add_span_processor(
            SimpleSpanProcessor(ConsoleSpanExporter())
        )

def grpcInstrumentClient():
    GrpcInstrumentorClient().instrument()

def grpcInstrumentServer():
    GrpcInstrumentorServer().instrument()

class Span:
    def __init__(self, name):
        self.name = name

    def __enter__(self):
        tracer = trace.get_tracer(__name__)
        span = tracer.start_span(self.name)
        self._otelSpan = trace.use_span(span, end_on_exit=True)
        with contextlib.ExitStack() as stack:
            stack.enter_context(self._otelSpan)
            self._stack = stack.pop_all()
        return self

    def __exit__(self, type, value, traceback):
        return self._stack.__exit__(type, value, traceback)
