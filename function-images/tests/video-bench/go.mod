module tests/video_analytics

go 1.16

replace tests/video_analytics/proto => ./proto

require (
	github.com/containerd/containerd v1.5.2
	github.com/ease-lab/vhive/examples/protobuf/helloworld v0.0.0-20210707110616-9ea18b3bc35e
	github.com/ease-lab/vhive/utils/tracing/go v0.0.0-20210708110826-fffc98ca29d6
	github.com/sirupsen/logrus v1.8.1
	google.golang.org/grpc v1.39.0
	google.golang.org/protobuf v1.27.1
)
