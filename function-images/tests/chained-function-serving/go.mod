module tests/chained-functions-serving

go 1.16

replace (
	github.com/ease-lab/vhive-xdt/proto/crossXDT => github.com/ease-lab/vhive-xdt/proto/crossXDT v0.0.0-20210809112617-4217d3a43d3c
	github.com/ease-lab/vhive-xdt/proto/downXDT => github.com/ease-lab/vhive-xdt/proto/downXDT v0.0.0-20210809112617-4217d3a43d3c
	github.com/ease-lab/vhive-xdt/proto/upXDT => github.com/ease-lab/vhive-xdt/proto/upXDT v0.0.0-20210809112617-4217d3a43d3c
	github.com/ease-lab/vhive-xdt/utils => github.com/ease-lab/vhive-xdt/utils v0.0.0-20210809112617-4217d3a43d3c
	github.com/ease-lab/vhive/utils/tracing/go => ../../../utils/tracing/go
	go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc => go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc v0.20.0
	tests/chained-functions-serving/proto => ./proto
)

require (
	github.com/aws/aws-sdk-go v1.15.11
	github.com/containerd/containerd v1.5.2
	github.com/ease-lab/vhive-xdt/sdk/golang v0.0.0-20210809112617-4217d3a43d3c
	github.com/ease-lab/vhive-xdt/utils v0.0.0-00010101000000-000000000000
	github.com/ease-lab/vhive/examples/protobuf/helloworld v0.0.0-20210630153229-45a1c5894f68
	github.com/ease-lab/vhive/utils/tracing/go v0.0.0-20210708110826-fffc98ca29d6
	github.com/sirupsen/logrus v1.8.1
	go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc v0.20.0
	google.golang.org/grpc v1.39.0
	google.golang.org/protobuf v1.27.1
)
