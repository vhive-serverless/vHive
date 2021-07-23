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

FROM vhiveease/python:latest as recogBuilder
WORKDIR /py
COPY ./function-images/tests/video-bench/requirements.txt ./
RUN apt update && \
    pip3 install --no-cache-dir --user torch==1.9.0+cpu torchvision==0.10.0+cpu torchaudio==0.9.0 -f https://download.pytorch.org/whl/torch_stable.html && \
    pip3 install --no-cache-dir --user -r requirements.txt
COPY ./utils/tracing/python/tracing.py ./
COPY ./function-images/tests/video-bench/object_recognition/* ./
COPY ./function-images/tests/video-bench/proto/videoservice_pb2_grpc.py ./
COPY ./function-images/tests/video-bench/proto/videoservice_pb2.py ./

FROM vhiveease/python-slim:latest as recog
COPY --from=recogBuilder /root/.local /root/.local
COPY --from=recogBuilder /py /app
WORKDIR /app
ENV PATH=/root/.local/bin:$PATH
ENTRYPOINT ["python3", "-u", "./recog.py"]


FROM vhiveease/python:latest as decodeBuilder
WORKDIR /py
COPY ./function-images/tests/video-bench/requirements.txt ./
RUN apt update && \
    apt-get install ffmpeg libsm6 libxext6 git -y && \
    pip3 install --no-cache-dir --user ffmpeg-python && \
    pip3 install --no-cache-dir --user opencv-python && \
    pip3 install --no-cache-dir --user -r requirements.txt
COPY ./utils/tracing/python/tracing.py ./
COPY ./function-images/tests/video-bench/decoder/decode.py ./
RUN mkdir frames
COPY ./function-images/tests/video-bench/proto/videoservice_pb2_grpc.py ./
COPY ./function-images/tests/video-bench/proto/videoservice_pb2.py ./
RUN --mount=type=secret,id=GOPRIVATE_KEY \
        git config --global url."https://ease-lab:$(cat /run/secrets/GOPRIVATE_KEY)@github.com/ease-lab/vhive-xdt".insteadOf "https://github.com/ease-lab/vhive-xdt"
RUN git clone -b python-sdk-use-metadata-at-dst --depth 1 https://github.com/ease-lab/vhive-xdt /xdt

FROM vhiveease/python-slim:latest as decode
RUN apt update && apt-get install ffmpeg libsm6 libxext6  -y
COPY --from=decodeBuilder /root/.local /root/.local
COPY --from=decodeBuilder /py /app
COPY --from=decodeBuilder /xdt/sdk/python/ /app
COPY --from=decodeBuilder /xdt/proto/downXDT/ /app
WORKDIR /app
ENV PATH=/root/.local/bin:$PATH
ENTRYPOINT ["python3", "-u", "./decode.py"]



FROM vhiveease/golang:latest as streamingBuilder
WORKDIR /app
RUN apk add --no-cache make git
RUN apk add -U --no-cache ca-certificates
COPY ./video_streaming/video-streaming.go ./video_streaming/
COPY ./video_streaming/reference ./reference
COPY ./go.mod ./
COPY ./go.sum ./
COPY ./proto ./proto
RUN --mount=type=secret,id=GOPRIVATE_KEY \
    git config --global url."https://ease-lab:$(cat /run/secrets/GOPRIVATE_KEY)@github.com/ease-lab/vhive-xdt".insteadOf "https://github.com/ease-lab/vhive-xdt"
RUN go env -w GOPRIVATE=github.com/ease-lab/vhive-xdt
RUN go mod download && \
   CGO_ENABLED=0 GOOS=linux go build -v -o ./video_streaming/streaming-bin ./video_streaming

FROM scratch as streaming
WORKDIR /app
COPY --from=streamingBuilder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=streamingBuilder /app/video_streaming/streaming-bin /app/gen/exe
COPY --from=streamingBuilder /app/reference /app/reference

ENTRYPOINT [ "/app/gen/exe" ]