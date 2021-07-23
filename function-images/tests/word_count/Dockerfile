FROM golang:1.16.6-buster AS builder
WORKDIR /word_count
COPY . .
RUN make

FROM debian:buster
COPY --from=builder /word_count/bin/word_count /usr/local/bin/word_count
ENTRYPOINT [ "/usr/local/bin/word_count" ]
