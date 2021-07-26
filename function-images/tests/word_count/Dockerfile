FROM vhiveease/golang-builder AS builder
WORKDIR /word_count
COPY . .
RUN make

FROM vhiveease/runner
RUN apt-get update && apt-get install -y ca-certificates
COPY --from=builder /word_count/bin/word_count /usr/local/bin/word_count
ENTRYPOINT [ "/usr/local/bin/word_count" ]
