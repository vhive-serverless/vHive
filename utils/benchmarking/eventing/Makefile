.PHONY: all clean image-timeseriesdb push-timeseriesdb

all: clean push-timeseriesdb

clean:
	find ./proto/ ! -name '*.proto' -type f -exec rm -f {} +
	rm -f bin/*

bin/:
	mkdir -p bin


bin/timeseriesdb: timeseriesdb/main.go bin/ proto/timeseries_grpc.pb.go proto/timeseries.pb.go
	go build -o bin/timeseriesdb eventing/timeseriesdb

TAG_TIMESERIESDB = vhiveease/tsdb-timeseriesdb:latest
image-timeseriesdb: Dockerfile bin/timeseriesdb
	docker build --tag ${TAG_TIMESERIESDB} --build-arg target_arg=timeseriesdb .

push-timeseriesdb: image-timeseriesdb
	docker push ${TAG_TIMESERIESDB}


proto/timeseries_grpc.pb.go proto/timeseries.pb.go: proto/timeseries.proto
	protoc -I ./proto/ \
		--go_out=proto \
		--go_opt="paths=source_relative" \
		--go-grpc_out=proto \
		--go-grpc_opt="paths=source_relative" \
		timeseries.proto
