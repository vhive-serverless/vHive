# MIT License
#
# Copyright (c) 2021 Michal Baczun and EASE lab
#
# Permission is hereby granted, free of charge, to any person obtaining a copy
# of this software and associated documentation files (the "Software"), to deal
# in the Software without restriction, including without limitation the rights
# to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
# copies of the Software, and to permit persons to whom the Software is
# furnished to do so, subject to the following conditions:
#
# The above copyright notice and this permission notice shall be included in all
# copies or substantial portions of the Software.
#
# THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
# IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
# FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
# AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
# LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
# OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
# SOFTWARE.

FROM vhiveease/golang:latest as builder

WORKDIR /app
RUN apk add --no-cache make
RUN apk add -U --no-cache ca-certificates
# We use ARG and ENV to have a single Dockerfile for the all three programs.
# https://vsupalov.com/docker-build-pass-environment-variables/
ARG target_arg
ENV target=$target_arg
RUN mkdir /log
COPY . .
WORKDIR /app/function-images/tests/chained-function-serving
RUN go env -w GOPRIVATE=github.com/ease-lab/vhive-xdt
RUN apk update && \
    apk add git
# Using buidkit secrets to pass sensitive info during the build.
# https://pythonspeed.com/articles/docker-build-secrets/
RUN --mount=type=secret,id=GOPRIVATE_KEY \
    git config --global url."https://ease-lab:$(cat /run/secrets/GOPRIVATE_KEY)@github.com/ease-lab/vhive-xdt".insteadOf "https://github.com/ease-lab/vhive-xdt"
RUN go mod tidy
RUN go mod download && \
   CGO_ENABLED=0 GOOS=linux go build -v -o ./${target}-bin ./${target}/

FROM scratch
WORKDIR /app
ARG target_arg
ENV target=$target_arg
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /log/ /app/log/ 
COPY --from=builder /app/function-images/tests/chained-function-serving/${target}-bin /app/exe
ENTRYPOINT [ "/app/exe" ]
