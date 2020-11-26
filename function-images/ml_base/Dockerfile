FROM alpine:3.9 as base

LABEL maintainer="Juliano Petronetto <juliano@petronetto.com.br>" \
      name="Alpine Machine Learning Base Container" \
      description="Alpine for Machine Learning/Deep Learning stuffs with Python" \
      url="https://hub.docker.com/r/petronetto/alpine-machine-learning-base" \
      vcs-url="https://github.com/petronetto/alpine-machine-learning-base" \
      vendor="Petronetto DevTech" \
      version="1.0"

RUN echo "|--> Updating" \
    && apk update && apk upgrade \
    && echo http://dl-cdn.alpinelinux.org/alpine/edge/main | tee /etc/apk/repositories \
    && echo http://dl-cdn.alpinelinux.org/alpine/edge/testing | tee -a /etc/apk/repositories \
    && echo http://dl-cdn.alpinelinux.org/alpine/edge/community | tee -a /etc/apk/repositories \
    && echo "|--> Install basics pre-requisites" \
    && apk add --no-cache tini \
        curl ca-certificates python3 py3-numpy py3-numpy-f2py \
        freetype jpeg libpng libstdc++ libgomp font-noto \
    && echo "|--> Install Python basics" \
    && python3 -m ensurepip \
    && rm -r /usr/lib/python*/ensurepip \
    && pip3 --no-cache-dir install --upgrade pip setuptools wheel \
    && if [ ! -e /usr/bin/pip ]; then ln -s pip3 /usr/bin/pip; fi \
    && if [[ ! -e /usr/bin/python ]]; then ln -sf /usr/bin/python3 /usr/bin/python; fi \
    && ln -s locale.h /usr/include/xlocale.h \
    && echo "|--> Install build dependencies" \
    && apk add --no-cache --virtual=.build-deps \
        build-base linux-headers python3-dev git cmake jpeg-dev bash \
        libffi-dev gfortran openblas-dev py-numpy-dev freetype-dev libpng-dev \
    && echo "|--> Install Python packages" \
    && pip install -U --no-cache-dir pyyaml pymkl cffi scikit-learn \
        matplotlib ipywidgets notebook requests pillow pandas seaborn protobuf==3.11.3 grpcio==1.26.0 \
    && echo "|--> Cleaning" \
    && rm /usr/include/xlocale.h \
    && rm -rf /root/.cache \
    && rm -rf /root/.[acpw]* \
    && rm -rf /var/cache/apk/* \
    && find /usr/lib/python3.6 -name __pycache__ | xargs rm -r \
    && echo "|--> Configure Jupyter extension" \
    && jupyter nbextension enable --py widgetsnbextension \
    && mkdir -p ~/.ipython/profile_default/startup/ \
    && echo "import warnings" >> ~/.ipython/profile_default/startup/config.py \
    && echo "warnings.filterwarnings('ignore')" >> ~/.ipython/profile_default/startup/config.py \
    && echo "c.NotebookApp.token = u''" >> ~/.ipython/profile_default/startup/config.py \
    && echo "|--> Done!"

ENTRYPOINT ["/sbin/tini", "--"]
