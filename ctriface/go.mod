module github.com/ustiugov/fccd-orchestrator/ctriface

go 1.13

require (
	github.com/containerd/containerd v1.3.3
	github.com/davecgh/go-spew v1.1.1
	github.com/firecracker-microvm/firecracker-containerd v0.0.0-20200331220105-afedbc74f5ee
	github.com/pkg/errors v0.9.1
	github.com/sirupsen/logrus v1.5.0
	github.com/stretchr/testify v1.5.1
	github.com/ustiugov/fccd-orchestrator v0.0.0-20200421101715-3d8808b0d980
	github.com/ustiugov/fccd-orchestrator/misc v0.0.0-20200421101715-3d8808b0d980
	google.golang.org/grpc v1.28.0
)

// Workaround for github.com/containerd/containerd issue #3031
replace github.com/docker/distribution v2.7.1+incompatible => github.com/docker/distribution v2.7.1-0.20190205005809-0d3efadf0154+incompatible
