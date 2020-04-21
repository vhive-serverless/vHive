module github.com/ustiugov/fccd-orchestrator

go 1.13

// Workaround for github.com/containerd/containerd issue #3031
replace github.com/docker/distribution v2.7.1+incompatible => github.com/docker/distribution v2.7.1-0.20190205005809-0d3efadf0154+incompatible

require (
	github.com/containerd/containerd v1.3.3
	github.com/golang/protobuf v1.3.3
	github.com/sirupsen/logrus v1.5.0
	github.com/stretchr/testify v1.5.1
	github.com/ustiugov/fccd-orchestrator/ctriface v0.0.0-20200421104402-f4bf733168e0
	github.com/ustiugov/fccd-orchestrator/misc v0.0.0-20200421104402-f4bf733168e0
	github.com/ustiugov/fccd-orchestrator/proto v0.0.0-20200421101715-3d8808b0d980
	google.golang.org/grpc v1.28.0
)
