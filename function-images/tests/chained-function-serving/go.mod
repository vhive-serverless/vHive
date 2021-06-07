module tests/chained-functions-serving

go 1.16

replace tests/chained-functions-serving/proto => ./proto

require (
	github.com/ease-lab/vhive/examples/protobuf/helloworld v0.0.0-20210608114032-dab7e310da45
	google.golang.org/grpc v1.38.0
	google.golang.org/protobuf v1.26.0
)
