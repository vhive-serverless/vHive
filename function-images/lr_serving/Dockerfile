#FROM python:3.8.1-slim as var_workload

FROM frolvlad/alpine-python3 as var_workload
RUN apk add --no-cache \
        --virtual=.build-dependencies \
        g++ gcc gfortran file binutils linux-headers \
        musl-dev python3-dev cython openblas-dev lapack-dev && \
    apk add libstdc++ openblas lapack && \
    \
    ln -s locale.h /usr/include/xlocale.h && \
    \
    pip install --disable-pip-version-check --no-build-isolation numpy==1.19.0 && \
    pip install --disable-pip-version-check --no-build-isolation pandas==1.0.5 && \
    pip install --disable-pip-version-check --no-build-isolation protobuf==3.11.3 && \
    pip install --disable-pip-version-check --no-build-isolation grpcio==1.26.0 && \
    pip install --disable-pip-version-check --no-build-isolation pybind11 && \
    \
    # scipy 1.4.x releases are broken on Alpine due to: https://github.com/scipy/scipy/issues/11319
    #pip install --disable-pip-version-check --no-build-isolation scipy && \
    apk add --no-cache --virtual=.build-dependencies-scipy-patch patch && \
    cd /tmp && \
    SCIPY_VERSION=1.4.1 && \
    wget "https://github.com/scipy/scipy/releases/download/v$SCIPY_VERSION/scipy-$SCIPY_VERSION.tar.xz" && \
    tar -xJf "scipy-$SCIPY_VERSION.tar.xz" && \
    (cd "scipy-$SCIPY_VERSION" && wget https://patch-diff.githubusercontent.com/raw/scipy/scipy/pull/11320.patch -O - | patch -p1) && \
    pip install --disable-pip-version-check --no-build-isolation "/tmp/scipy-$SCIPY_VERSION/" && \
    rm -rf /tmp/* && \
    apk del .build-dependencies-scipy-patch && \
    \
    pip install --disable-pip-version-check --no-build-isolation scikit-learn && \
    \
    rm -r /root/.cache && \
    find /usr/lib/python3.*/ -name 'tests' -exec rm -r '{}' + && \
    find /usr/lib/python3.*/site-packages/ -name '*.so' -print -exec sh -c 'file "{}" | grep -q "not stripped" && strip -s "{}"' \; && \
    \
    rm /usr/include/xlocale.h && \
    \
    apk del .build-dependencies

# Add pycddlib and cvxopt with GLPK
RUN cd /tmp && \
    apk add --no-cache \
        --virtual=.build-dependencies \
        gcc make file binutils \
        musl-dev python3-dev cython gmp-dev suitesparse-dev openblas-dev && \
    apk add gmp suitesparse && \
    \
    pip install --disable-pip-version-check --no-build-isolation pycddlib && \
    \
    wget "ftp://ftp.gnu.org/gnu/glpk/glpk-4.65.tar.gz" && \
    tar xzf "glpk-4.65.tar.gz" && \
    cd "glpk-4.65" && \
    ./configure --disable-static && \
    make -j4 && \
    make install-strip && \
    CVXOPT_BLAS_LIB=openblas CVXOPT_LAPACK_LIB=openblas CVXOPT_BUILD_GLPK=1 pip install --disable-pip-version-check --no-build-isolation --global-option=build_ext --global-option="-I/usr/include/suitesparse" cvxopt && \
    \
    rm -r /root/.cache && \
    find /usr/lib/python3.*/site-packages/ -name '*.so' -print -exec sh -c 'file "{}" | grep -q "not stripped" && strip -s "{}"' \; && \
    \
    apk del .build-dependencies && \
    rm -rf /tmp/*


COPY requirements.txt .
RUN pip3 install --no-cache-dir -r requirements.txt

COPY *.py /
COPY dataset.csv /
COPY lr_model.pk /

EXPOSE 50051

STOPSIGNAL SIGKILL

CMD ["/usr/bin/python", "/server.py"]
