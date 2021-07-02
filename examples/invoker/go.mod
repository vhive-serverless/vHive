module github.com/ease-lab/vhive/examples/invoker

go 1.16

replace (
	eventing => ../../utils/benchmarking/eventing
	github.com/ease-lab/vhive/examples/endpoint => ../endpoint
	github.com/ease-lab/vhive/utils/tracing/go => ../../utils/tracing/go
	go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc => go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc v0.20.0
)

require (
	eventing v0.0.0-00010101000000-000000000000
	github.com/containerd/containerd v1.5.2
	github.com/ease-lab/vhive/examples/endpoint v0.0.0-00010101000000-000000000000
	github.com/ease-lab/vhive/examples/protobuf/helloworld v0.0.0-20210621160829-cea81c4fff31
	github.com/ease-lab/vhive/utils/tracing/go v0.0.0-00010101000000-000000000000
	github.com/golang/protobuf v1.5.2
	github.com/google/uuid v1.2.0 // indirect
	github.com/sirupsen/logrus v1.8.1
	go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc v0.20.0
	google.golang.org/grpc v1.38.0
	google.golang.org/protobuf v1.26.0 // indirect
)
