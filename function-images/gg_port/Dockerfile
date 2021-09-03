FROM vhiveease/py_grpc_ubuntu_slim:builder_grpc as builder_workload
COPY --from=vhiveease/py_grpc:builder_grpc /root/.local /root/.local
COPY requirements.txt .
RUN pip3 install --user -r requirements.txt

FROM vhiveease/py_grpc_ubuntu_slim:base as var_workload
COPY *.py /
COPY common /common
COPY gg-execute-static /gg-execute-static
COPY --from=builder_workload /root/.local /root/.local
ENV APP_HOME /app
WORKDIR $APP_HOME
EXPOSE 50051

STOPSIGNAL SIGKILL

CMD ["/usr/local/bin/python", "/server.py"]
