module tests/chained-functions-serving

go 1.16

replace (
	github.com/ease-lab/vhive/utils/tracing/go => ./utils/tracing/go
	tests/chained-functions-serving/proto => ./proto
)

require (
	github.com/containerd/containerd v1.5.2
	github.com/ease-lab/vhive/examples/protobuf/helloworld v0.0.0-20210630153229-45a1c5894f68
	github.com/ease-lab/vhive/utils/tracing/go v0.0.0-20210630153229-45a1c5894f68
	github.com/sirupsen/logrus v1.8.1
	google.golang.org/grpc v1.39.0
	google.golang.org/protobuf v1.27.1
)
