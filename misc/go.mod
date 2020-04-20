module github.com/ustiugov/fccd-orchestrator/misc

go 1.13

// Workaround for github.com/containerd/containerd issue #3031
replace github.com/docker/distribution v2.7.1+incompatible => github.com/docker/distribution v2.7.1-0.20190205005809-0d3efadf0154+incompatible

require (
	github.com/containerd/containerd v1.3.4
	github.com/pkg/errors v0.9.1
	github.com/sirupsen/logrus v1.5.0
	github.com/ustiugov/fccd-orchestrator v0.0.0-20200416171230-6533745bd3b1
	google.golang.org/grpc v1.28.1
)
