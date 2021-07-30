module tests/tracing/python/client-server

go 1.16

replace go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc => go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc v0.20.0

require (
	github.com/containerd/containerd v1.5.4
	github.com/ease-lab/vhive/examples/protobuf/helloworld v0.0.0-20210629083624-1f3cea290c54
	github.com/ease-lab/vhive/utils/tracing/go v0.0.0-20210629083624-1f3cea290c54
	github.com/sirupsen/logrus v1.8.1
	go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc v0.20.0
	google.golang.org/grpc v1.38.0
)
