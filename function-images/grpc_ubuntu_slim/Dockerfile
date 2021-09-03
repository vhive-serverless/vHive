FROM python:3.9-slim-bullseye as base

FROM base as builder_grpc
ENV GRPC_PYTHON_VERSION 1.26.0
RUN apt update && \
    apt install -y gcc g++ && \
    pip3 install --user protobuf==3.11.3 grpcio==${GRPC_PYTHON_VERSION}
    # tools are not required for running
    #grpcio-tools==${GRPC_PYTHON_VERSION}
