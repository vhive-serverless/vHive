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

FROM amd64/golang:1.16.4-alpine3.13 as consumerBuilder

WORKDIR /app
COPY go.mod /app/go.mod
COPY consumer/ /app/consumer/
COPY proto/ /app/proto/
RUN go mod download && \
   CGO_ENABLED=0 GOOS=linux go build -v -o /main /app/consumer/consumer.go
RUN mkdir /log

FROM amd64/golang:1.16.4-alpine3.13 as consumer
WORKDIR /
COPY --from=consumerBuilder /main /main
COPY --from=consumerBuilder /log/ /log/
ENTRYPOINT [ "/main" ] 

FROM amd64/golang:1.16.4-alpine3.13 as producerBuilder

WORKDIR /app
COPY go.mod /app/go.mod
COPY producer/ /app/producer/
COPY proto/ /app/proto/
RUN go mod download && \
   CGO_ENABLED=0 GOOS=linux go build -v -o /main /app/producer/producer.go
RUN mkdir /log

FROM amd64/golang:1.16.4-alpine3.13 as producer
WORKDIR /
COPY --from=producerBuilder /main /main
COPY --from=producerBuilder /log/ /log/
ENTRYPOINT [ "/main" ] 

FROM amd64/golang:1.16.4-alpine3.13 as lite-producerBuilder

WORKDIR /app
COPY go.mod /app/go.mod
COPY lite-producer/ /app/lite-producer/
COPY proto/ /app/proto/
RUN go mod download && \
   CGO_ENABLED=0 GOOS=linux go build -v -o /main /app/lite-producer/lite-producer.go
RUN mkdir /log

FROM amd64/golang:1.16.4-alpine3.13 as lite-producer
WORKDIR /
COPY --from=lite-producerBuilder /main /main
COPY --from=lite-producerBuilder /log/ /log/
ENTRYPOINT [ "/main" ] 