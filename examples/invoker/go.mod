module github.com/vhive-serverless/vhive/examples/invoker

go 1.19

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
	google.golang.org/grpc v1.47.0
	google.golang.org/protobuf v1.31.0
)

require (
	github.com/openzipkin/zipkin-go v0.2.5 // indirect
	go.opentelemetry.io/contrib v0.20.0 // indirect
	go.opentelemetry.io/otel v0.20.0 // indirect
	go.opentelemetry.io/otel/exporters/trace/zipkin v0.20.0 // indirect
	go.opentelemetry.io/otel/metric v0.20.0 // indirect
	go.opentelemetry.io/otel/sdk v0.20.0 // indirect
	go.opentelemetry.io/otel/trace v0.20.0 // indirect
	golang.org/x/net v0.0.0-20220722155237-a158d28d115b // indirect
	golang.org/x/sys v0.0.0-20220722155257-8c9f86f7a55f // indirect
	golang.org/x/text v0.3.7 // indirect
	google.golang.org/genproto v0.0.0-20220502173005-c8bf987b8c21 // indirect
)
