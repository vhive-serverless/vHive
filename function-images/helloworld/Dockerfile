# FROM vhiveease/py_grpc:builder_grpc as builder_workload
# COPY --from=vhiveease/py_grpc:builder_grpc /root/.local /root/.local
# COPY requirements.txt .
# RUN pip3 install --user -r requirements.txt

# FROM vhiveease/py_grpc:base as var_workload
# COPY *.py /
# COPY --from=builder_workload /root/.local /root/.local
# RUN apk add libstdc++ --update --no-cache

FROM python:3.7-slim

RUN pip install protobuf grpcio

ENV PORT 50051
EXPOSE PORT

STOPSIGNAL SIGKILL

CMD ["/usr/local/bin/python", "/server.py", "-p $PORT"]
