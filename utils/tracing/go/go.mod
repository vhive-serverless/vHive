module github.com/vhive-serverless/vhive/utils/tracing/go

go 1.19

require (
	github.com/containerd/containerd v1.5.2
	github.com/sirupsen/logrus v1.8.1
	go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc v0.20.0
	go.opentelemetry.io/otel v0.20.0
	go.opentelemetry.io/otel/exporters/trace/zipkin v0.20.0
	go.opentelemetry.io/otel/sdk v0.20.0
	go.opentelemetry.io/otel/trace v0.20.0
	google.golang.org/grpc v1.58.3
)

require (
	cloud.google.com/go/compute/metadata v0.2.3 // indirect
	github.com/golang/protobuf v1.5.3 // indirect
	github.com/openzipkin/zipkin-go v0.2.5 // indirect
	go.opentelemetry.io/contrib v0.20.0 // indirect
	go.opentelemetry.io/otel/metric v0.20.0 // indirect
	golang.org/x/net v0.12.0 // indirect
	golang.org/x/sys v0.10.0 // indirect
	golang.org/x/text v0.11.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20230711160842-782d3b101e98 // indirect
	google.golang.org/protobuf v1.31.0 // indirect
)
