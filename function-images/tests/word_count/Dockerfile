FROM vhiveease/golang-builder AS builder
WORKDIR /vhive
COPY . .
WORKDIR /vhive/function-images/tests/word_count
RUN make

FROM vhiveease/runner
RUN apt-get update && apt-get install -y ca-certificates
COPY --from=builder /vhive/function-images/tests/word_count/bin/word_count /usr/local/bin/word_count
ENTRYPOINT [ "/usr/local/bin/word_count" ]
