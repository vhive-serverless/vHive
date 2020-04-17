module github.com/ustiugov/fccd-orchestrator/ctriface

go 1.14

require (
	github.com/containerd/containerd v1.3.4
	github.com/davecgh/go-spew v1.1.1
	github.com/firecracker-microvm/firecracker-containerd v0.0.0-20200331220105-afedbc74f5ee
	github.com/pkg/errors v0.9.1
	github.com/sirupsen/logrus v1.5.0
	github.com/stretchr/testify v1.5.1
	github.com/ustiugov/fccd-orchestrator v0.0.0-20200416171230-6533745bd3b1
	github.com/ustiugov/fccd-orchestrator/misc v0.0.0-20200417111927-c9f33db90a77
	google.golang.org/grpc v1.28.1
)

// Workaround for github.com/containerd/containerd issue #3031
replace github.com/docker/distribution v2.7.1+incompatible => github.com/docker/distribution v2.7.1-0.20190205005809-0d3efadf0154+incompatible
