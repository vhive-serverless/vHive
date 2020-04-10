module github.com/ustiugov/fccd-orchestrator/misc

go 1.13

require (
	github.com/containerd/containerd v1.3.3
	github.com/pkg/errors v0.9.1
	github.com/ustiugov/fccd-orchestrator v0.0.0-20200410124934-5c549f460418
	google.golang.org/grpc v1.28.1
)

// Workaround for github.com/containerd/containerd issue #3031
replace github.com/docker/distribution v2.7.1+incompatible => github.com/docker/distribution v2.7.1-0.20190205005809-0d3efadf0154+incompatible
