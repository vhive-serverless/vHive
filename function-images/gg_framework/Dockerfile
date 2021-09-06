FROM python:3.9-slim-bullseye as var_workload
ENV GRPC_PYTHON_VERSION 1.26.0
RUN apt update && \
    apt install -y gcc g++ && \
    pip3 install --user protobuf==3.11.3 grpcio==${GRPC_PYTHON_VERSION}

COPY requirements.txt .
RUN pip3 install --user -r requirements.txt

COPY *.py /
COPY common /common
COPY gg-execute-static /gg-execute-static
ENV APP_HOME /app
WORKDIR $APP_HOME
EXPOSE 50051

STOPSIGNAL SIGKILL

CMD ["/usr/local/bin/python", "/server.py"]
