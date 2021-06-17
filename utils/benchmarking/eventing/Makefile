.PHONY: all clean image-relay image-timeseriesdb push-relay push-timeseriesdb

all: clean bin/tscli push-relay push-timeseriesdb

clean:
	find ./proto/ ! -name '*.proto' -type f -exec rm -f {} +
	rm -f bin/*

bin/:
	mkdir -p bin


bin/relay: relay/main.go bin/ proto/timeseries_grpc.pb.go proto/timeseries.pb.go
	go build -o bin/relay github.com/ease-lab/vhive/utils/benchmarking/eventing/relay

TAG_RELAY = vhiveease/tsdb-relay:latest
image-relay: Dockerfile bin/relay
	docker build --tag ${TAG_RELAY} --build-arg target_arg=relay .

push-relay: image-relay
	docker push ${TAG_RELAY}


bin/timeseriesdb: timeseriesdb/main.go bin/ proto/timeseries_grpc.pb.go proto/timeseries.pb.go
	go build -o bin/timeseriesdb github.com/ease-lab/vhive/utils/benchmarking/eventing/timeseriesdb

TAG_TIMESERIESDB = vhiveease/tsdb-timeseriesdb:latest
image-timeseriesdb: Dockerfile bin/relay
	docker build --tag ${TAG_TIMESERIESDB} --build-arg target_arg=timeseriesdb .

push-timeseriesdb: image-timeseriesdb
	docker push ${TAG_TIMESERIESDB}


bin/tscli: tscli/main.go bin/ proto/timeseries_grpc.pb.go proto/timeseries.pb.go
	go build -o bin/tscli github.com/ease-lab/vhive/utils/benchmarking/eventing/tscli

proto/timeseries_grpc.pb.go proto/timeseries.pb.go: proto/timeseries.proto
	protoc -I ./proto/ \
		--go_out=proto \
		--go_opt="paths=source_relative" \
		--go-grpc_out=proto \
		--go-grpc_opt="paths=source_relative" \
		timeseries.proto
