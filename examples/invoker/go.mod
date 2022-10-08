module github.com/vhive-serverless/vhive/examples/invoker

go 1.16

replace (
	github.com/vhive-serverless/vhive/examples/endpoint => ../endpoint
	github.com/vhive-serverless/vhive/utils/benchmarking/eventing => ../../utils/benchmarking/eventing
	github.com/vhive-serverless/vhive/utils/tracing/go => ../../utils/tracing/go
	go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc => go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc v0.20.0
)

require (
	github.com/containerd/containerd v1.5.2
	github.com/golang/protobuf v1.5.2
	github.com/google/uuid v1.2.0
	github.com/sirupsen/logrus v1.8.1
	github.com/vhive-serverless/vhive/examples/endpoint v0.0.0-00010101000000-000000000000
	github.com/vhive-serverless/vhive/utils/benchmarking/eventing v0.0.0-00010101000000-000000000000
	github.com/vhive-serverless/vhive/utils/tracing/go v0.0.0-00010101000000-000000000000
	go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc v0.20.0
	google.golang.org/grpc v1.50.0
	google.golang.org/protobuf v1.27.1
)
