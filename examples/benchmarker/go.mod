module github.com/ease-lab/vhive/examples/benchmarker

go 1.16

replace (
	github.com/ease-lab/vhive/utils/tracing/go => ../../utils/tracing/go
	go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc => go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc v0.20.0
)

require (
	github.com/containerd/containerd v1.5.4
	github.com/creasty/defaults v1.5.1
	github.com/ease-lab/vhive/examples/protobuf/helloworld v0.0.0-20210726194144-23534d39facf
	github.com/ease-lab/vhive/utils/tracing/go v0.0.0-00010101000000-000000000000
	github.com/ghodss/yaml v1.0.0
	github.com/sirupsen/logrus v1.8.1
	go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc v0.20.0
	gonum.org/v1/gonum v0.9.3
	gonum.org/v1/plot v0.9.0
	google.golang.org/grpc v1.39.0
)
