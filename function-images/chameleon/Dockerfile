FROM ustiugov/py_grpc:builder_grpc as builder_workload
COPY --from=ustiugov/py_grpc:builder_grpc /root/.local /root/.local
COPY requirements.txt .
RUN pip3 install --user -r requirements.txt

FROM ustiugov/py_grpc:base as var_workload
COPY *.py /
COPY --from=builder_workload /root/.local /root/.local
RUN apk add libstdc++ --update --no-cache

EXPOSE 50051

STOPSIGNAL SIGKILL

CMD ["/usr/local/bin/python", "/server.py"]
