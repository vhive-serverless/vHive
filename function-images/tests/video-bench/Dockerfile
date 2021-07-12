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

FROM python:3.8 as recogBuilder
WORKDIR /py
COPY ./function-images/tests/video-bench/requirements.txt ./
RUN apt update && \
    pip3 install --no-cache-dir --user torch==1.8.1+cpu torchvision==0.9.1+cpu torchaudio==0.8.1 -f https://download.pytorch.org/whl/torch_stable.html && \
    pip3 install --no-cache-dir --user grpcio && \
    pip3 install --no-cache-dir --user -r requirements.txt && \
    pip3 install --no-cache-dir --user grpcio-tools
COPY ./utils/tracing/python/tracing.py ./
COPY ./function-images/tests/video-bench/object_recognition/* ./
COPY ./function-images/tests/video-bench/proto/videoservice_pb2_grpc.py ./
COPY ./function-images/tests/video-bench/proto/videoservice_pb2.py ./

FROM python:3.8-slim as recog
COPY --from=recogBuilder /root/.local /root/.local
COPY --from=recogBuilder /py /app
WORKDIR /app
ENV PATH=/root/.local/bin:$PATH
ENTRYPOINT ["python3", "-u", "./recog.py"]


FROM python:3.8 as decodeBuilder
WORKDIR /py
COPY ./function-images/tests/video-bench/requirements.txt ./
RUN apt update && \
    apt-get install ffmpeg libsm6 libxext6  -y && \
    pip3 install --no-cache-dir --user opencv-python && \
    pip3 install --no-cache-dir --user grpcio && \
    pip3 install --no-cache-dir --user -r requirements.txt && \
    pip3 install --no-cache-dir --user grpcio-tools
COPY ./utils/tracing/python/tracing.py ./
COPY ./function-images/tests/video-bench/decoder/decode.py ./
RUN mkdir frames
COPY ./function-images/tests/video-bench/proto/videoservice_pb2_grpc.py ./
COPY ./function-images/tests/video-bench/proto/videoservice_pb2.py ./

FROM python:3.8-slim as decode
RUN apt update && apt-get install ffmpeg libsm6 libxext6  -y
COPY --from=decodeBuilder /root/.local /root/.local
COPY --from=decodeBuilder /py /app
WORKDIR /app
ENV PATH=/root/.local/bin:$PATH
ENTRYPOINT ["python3", "-u", "./decode.py"]



FROM vhiveease/golang:latest as generatorBuilder
WORKDIR /app
RUN apk add --no-cache make
COPY ./video_generator/video-streaming.go ./video_generator/
COPY ./video_generator/reference ./reference
COPY ./go.mod ./
COPY ./go.sum ./
COPY ./proto ./proto
RUN go mod download && \
   CGO_ENABLED=0 GOOS=linux go build -v -o ./video_generator/generator-bin ./video_generator

FROM scratch as generator
WORKDIR /app
COPY --from=generatorBuilder /app/video_generator/generator-bin /app/gen/exe
COPY --from=generatorBuilder /app/reference /app/reference

ENTRYPOINT [ "/app/gen/exe" ]