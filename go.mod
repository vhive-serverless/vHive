module github.com/ustiugov/fccd-orchestrator

go 1.13

require (
	github.com/containerd/containerd v1.3.3
	github.com/firecracker-microvm/firecracker-containerd v0.0.0-20200331220105-afedbc74f5ee
	github.com/golang/protobuf v1.3.5
	github.com/pkg/errors v0.9.1
	github.com/sirupsen/logrus v1.5.0
	github.com/ustiugov/fccd-orchestrator/ctriface v0.0.0-20200413123238-4ab57613a093
	github.com/ustiugov/fccd-orchestrator/ctriface v0.0.0-20200414201630-23eb647b8b12
	github.com/ustiugov/fccd-orchestrator/misc v0.0.0-20200414195918-0dd99bd7b46d
	github.com/ustiugov/fccd-orchestrator/proto v0.0.0-20200330172127-0d359e57cf32
	github.com/ustiugov/skv v0.0.0-20180909015525-9def2caac4cc
	google.golang.org/grpc v1.28.1
)

// Workaround for github.com/containerd/containerd issue #3031
replace github.com/docker/distribution v2.7.1+incompatible => github.com/docker/distribution v2.7.1-0.20190205005809-0d3efadf0154+incompatible
