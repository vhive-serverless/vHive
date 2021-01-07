FROM denismakogon/opencv3-slim:edge as var_workload

RUN apt update && apt -y install libgl1-mesa-glx libglib2.0-0
RUN pip3 install --upgrade pip && pip3 install minio && pip3 install grpcio && pip3 install opencv-python && pip3 install protobuf

COPY *.py /

EXPOSE 50051

STOPSIGNAL SIGKILL

CMD ["/usr/local/bin/python3", "/server.py"]
