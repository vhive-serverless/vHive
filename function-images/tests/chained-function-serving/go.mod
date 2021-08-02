module tests/chained-functions-serving

go 1.16

replace (
	github.com/ease-lab/vhive-xdt/proto/downXDT => github.com/ease-lab/vhive-xdt/proto/downXDT v0.0.0-20210715085759-292eab4d9c31
	github.com/ease-lab/vhive-xdt/proto/upXDT => github.com/ease-lab/vhive-xdt/proto/upXDT v0.0.0-20210715085759-292eab4d9c31
	github.com/ease-lab/vhive-xdt/utils => github.com/ease-lab/vhive-xdt/utils v0.0.0-20210715085759-292eab4d9c31
	github.com/ease-lab/vhive/utils/tracing/go => ../../../utils/tracing/go
	tests/chained-functions-serving/proto => ./proto
)

require (
	github.com/containerd/containerd v1.5.2
	github.com/ease-lab/vhive-xdt/sdk/golang v0.0.0-20210715085759-292eab4d9c31
	github.com/ease-lab/vhive-xdt/utils v0.0.0-00010101000000-000000000000
	github.com/ease-lab/vhive/examples/protobuf/helloworld v0.0.0-20210630153229-45a1c5894f68
	github.com/ease-lab/vhive/utils/tracing/go v0.0.0-20210708110826-fffc98ca29d6
	github.com/sirupsen/logrus v1.8.1
	google.golang.org/grpc v1.39.0
	google.golang.org/protobuf v1.27.1
)
