module github.com/ustiugov/fccd-orchestrator

go 1.13

// Workaround for github.com/containerd/containerd issue #3031
replace github.com/docker/distribution v2.7.1+incompatible => github.com/docker/distribution v2.7.1-0.20190205005809-0d3efadf0154+incompatible

replace github.com/firecracker-microvm/firecracker-containerd => github.com/ustiugov/firecracker-containerd v0.0.0-20200617123207-cff37815af4e

require (
	github.com/containerd/containerd v1.3.5-0.20200521195814-e655edce10c9
	github.com/golang/protobuf v1.3.3
	github.com/sirupsen/logrus v1.5.0
	github.com/stretchr/testify v1.5.1
	github.com/ustiugov/fccd-orchestrator/ctriface v0.0.0-20200617143218-c9b3c238ebd2
	github.com/ustiugov/fccd-orchestrator/misc v0.0.0-20200608162316-88962af36173
	github.com/ustiugov/fccd-orchestrator/proto v0.0.0-20200421101715-3d8808b0d980
	golang.org/x/sync v0.0.0-20200317015054-43a5402ce75a
	google.golang.org/grpc v1.28.0
)
