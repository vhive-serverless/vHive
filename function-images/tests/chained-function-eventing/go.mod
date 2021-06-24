module eventing

go 1.16

replace github.com/ease-lab/vhive/utils/tracing/go => ../../../utils/tracing/go

require (
	github.com/cloudevents/sdk-go/observability/opencensus/v2 v2.4.1
	github.com/cloudevents/sdk-go/v2 v2.4.1
	github.com/containerd/containerd v1.5.2
	github.com/ease-lab/vhive/utils/tracing/go v0.0.0-00010101000000-000000000000
	github.com/golang/protobuf v1.5.2 // indirect
	github.com/google/uuid v1.2.0
	github.com/kelseyhightower/envconfig v1.4.0
	go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc v0.20.0
	google.golang.org/grpc v1.38.0
	google.golang.org/protobuf v1.26.0
)
