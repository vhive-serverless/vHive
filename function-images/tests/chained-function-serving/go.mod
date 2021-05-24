module tests/chained-functions-serving

go 1.16

replace tests/chained-functions-serving/proto => ./proto

require (
	google.golang.org/grpc v1.38.0
	google.golang.org/protobuf v1.26.0
)
