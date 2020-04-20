module github.com/ustiugov/fccd-orchestrator

go 1.14

// Workaround for github.com/containerd/containerd issue #3031
replace github.com/docker/distribution v2.7.1+incompatible => github.com/docker/distribution v2.7.1-0.20190205005809-0d3efadf0154+incompatible

require (
	github.com/containerd/containerd v1.3.4
	github.com/golang/protobuf v1.4.0
	github.com/pkg/errors v0.9.1
	github.com/sirupsen/logrus v1.5.0
	github.com/stretchr/testify v1.5.1
	github.com/ustiugov/fccd-orchestrator/ctriface v0.0.0-20200420204514-a5fcc6bd6e6f
	github.com/ustiugov/fccd-orchestrator/misc v0.0.0-20200417191122-72eef2f2433e
	github.com/ustiugov/fccd-orchestrator/proto v0.0.0-20200416161917-c9cd3cf6dbcf
	google.golang.org/grpc v1.28.1
)
