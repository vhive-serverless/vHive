module tests/chained-functions-serving

go 1.16

replace (
	github.com/ease-lab/vhive/utils/tracing/go => /app/utils/tracing
	tests/chained-functions-serving/proto => ./proto
)

require (
	github.com/ease-lab/vhive/examples/protobuf/helloworld v0.0.0-20210608114032-dab7e310da45
	github.com/ease-lab/vhive/utils/tracing/go v0.0.0-00010101000000-000000000000
	go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc v0.20.0
	go.opentelemetry.io/otel v0.20.0
	go.opentelemetry.io/otel/exporters/trace/zipkin v0.20.0
	go.opentelemetry.io/otel/sdk v0.20.0
	google.golang.org/grpc v1.38.0
	google.golang.org/protobuf v1.26.0
)
