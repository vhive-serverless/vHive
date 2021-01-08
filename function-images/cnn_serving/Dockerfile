FROM tatsushid/alpine-py3-tensorflow-jupyter as var_workload
COPY requirements.txt .
ENV GRPC_PYTHON_VERSION 1.26.0

RUN apk update && \
    apk add python3 python3-dev py3-pip && \
    ln -s /usr/bin/pip3 /usr/bin/pip && \
    ln -sf /usr/bin/pip3 /usr/local/bin/pip && \
    ln -sf /usr/bin/python3 /usr/local/bin/python && \
    ln -sf /usr/bin/python3 /usr/local/bin/python3 && \
    ln -s /usr/include/locale.h /usr/include/xlocale.h && \
    echo "http://dl-cdn.alpinelinux.org/alpine/edge/community" >> /etc/apk/repositories && \
    echo "http://dl-cdn.alpinelinux.org/alpine/edge/testing" >> /etc/apk/repositories && \
    apk update && \
    apk add --upgrade && \
    apk add --update --no-cache build-base gcc g++ protobuf && \
    apk add --allow-untrusted --repository http://dl-3.alpinelinux.org/alpine/edge/testing hdf5 hdf5-dev && \
    apk add py3-numpy && \
    apk add py-numpy-dev && \
    pip3 install --upgrade pip && \
    pip3 uninstall -y enum34 && \
    pip3 install --no-cache-dir Cython && \
    pip3 install --no-cache-dir -r requirements.txt && \
    pip3 install --no-cache-dir protobuf==3.11.3 grpcio==${GRPC_PYTHON_VERSION}

COPY *.py /
COPY image* /
COPY squeezenet_weights_tf_dim_ordering_tf_kernels.h5  /tmp

EXPOSE 50051

STOPSIGNAL SIGKILL

CMD ["/usr/bin/python", "/server.py"]
