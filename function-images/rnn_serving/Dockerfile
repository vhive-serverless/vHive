FROM petronetto/pytorch-alpine as var_workload
ENV GRPC_PYTHON_VERSION 1.26.0
COPY requirements.txt /

RUN apk update && \
    apk add --update --no-cache gcc g++ protobuf && \
    pip install --no-cache-dir protobuf==3.11.3 grpcio==${GRPC_PYTHON_VERSION} \
    pip install --no-cache-dir -r /requirements.txt

COPY *.py /
COPY rnn* /

EXPOSE 50051

STOPSIGNAL SIGKILL

CMD ["/usr/bin/python", "/server.py"]
