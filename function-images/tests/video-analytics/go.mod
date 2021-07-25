module tests/video_analytics

go 1.16

replace (
	github.com/ease-lab/vhive-xdt/proto/downXDT => github.com/ease-lab/vhive-xdt/proto/downXDT v0.0.0-20210723175119-4d291cd51959
	github.com/ease-lab/vhive-xdt/proto/upXDT => github.com/ease-lab/vhive-xdt/proto/upXDT v0.0.0-20210723175119-4d291cd51959
	github.com/ease-lab/vhive-xdt/utils => github.com/ease-lab/vhive-xdt/utils v0.0.0-20210723175119-4d291cd51959
	tests/video_analytics/proto => ./proto
)

require (
	github.com/aws/aws-sdk-go v1.15.11
	github.com/containerd/containerd v1.5.2
	github.com/ease-lab/vhive-xdt/sdk/golang v0.0.0-20210723175119-4d291cd51959
	github.com/ease-lab/vhive-xdt/utils v0.0.0-00010101000000-000000000000
	github.com/ease-lab/vhive/examples/protobuf/helloworld v0.0.0-20210707110616-9ea18b3bc35e
	github.com/ease-lab/vhive/utils/tracing/go v0.0.0-20210708110826-fffc98ca29d6
	github.com/sirupsen/logrus v1.8.1
	google.golang.org/grpc v1.39.0
	google.golang.org/protobuf v1.27.1
)
