module tests/chained-functions-serving

go 1.16

replace (
	go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc => go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc v0.20.0
	tests/chained-functions-serving/proto => ./proto
)

require (
	github.com/containerd/containerd v1.5.2
	github.com/ease-lab/vhive/examples/protobuf/helloworld v0.0.0-20210608114032-dab7e310da45
	github.com/ease-lab/vhive/utils/tracing/go v0.0.0-20210629083624-1f3cea290c54
	github.com/sirupsen/logrus v1.8.1
	go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc v0.21.0
	google.golang.org/grpc v1.38.0
	google.golang.org/protobuf v1.26.0
)
