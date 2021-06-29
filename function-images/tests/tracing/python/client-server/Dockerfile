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

#FROM vhiveease/py_grpc:builder_grpc as builder_workload
#COPY --from=vhiveease/py_grpc:builder_grpc /root/.local /root/.local
#COPY ./function-images/tests/tracing/python/client-server/requirements.txt .
#RUN pip3 install --upgrade pip
#RUN python3 -m pip install --upgrade setuptools
#RUN pip3 install --user -r requirements.txt

#FROM vhiveease/py_grpc:base as var_workload
#COPY ./utils/tracing/python/tracing.py /
#COPY ./function-images/tests/tracing/python/client-server/*.py /
#COPY --from=builder_workload /root/.local /root/.local
#RUN apk add libstdc++ --update --no-cache

#EXPOSE 50051

#STOPSIGNAL SIGKILL

#ENTRYPOINT ["/usr/local/bin/python", "/server.py"]

FROM python:3.8-slim-buster as server
COPY ./function-images/tests/tracing/python/client-server/requirements.txt .
COPY ./function-images/tests/tracing/python/client-server/*.py /
COPY ./utils/tracing/python/tracing.py /
RUN pip3 install --user -r requirements.txt

EXPOSE 50051

STOPSIGNAL SIGKILL

ENTRYPOINT ["python3", "-u", "/server.py"]

FROM python:3.8-slim-buster as client
COPY ./function-images/tests/tracing/python/client-server/requirements.txt .
COPY ./function-images/tests/tracing/python/client-server/*.py /
COPY ./utils/tracing/python/tracing.py /
RUN pip3 install --user -r requirements.txt

EXPOSE 50051

STOPSIGNAL SIGKILL

ENTRYPOINT ["python3", "-u", "/greeter_client.py"]

