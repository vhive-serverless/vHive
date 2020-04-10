module github.com/ustiugov/fccd-orchestrator/ctrIface

go 1.13

require (
	github.com/containerd/containerd v1.3.3
	github.com/davecgh/go-spew v1.1.1
	github.com/firecracker-microvm/firecracker-containerd v0.0.0-20200324214552-7383119704ec
	github.com/pkg/errors v0.9.1
	github.com/ustiugov/fccd-orchestrator v0.0.0-20200327173131-84041f0798f2
	github.com/ustiugov/fccd-orchestrator/misc v0.0.0-20200410111525-19cc7325fa09
	github.com/ustiugov/fccd-orchestrator/proto v0.0.0-20200327173131-84041f0798f2 // indirect
	google.golang.org/grpc v1.28.0
)

// Workaround for github.com/containerd/containerd issue #3031
replace github.com/docker/distribution v2.7.1+incompatible => github.com/docker/distribution v2.7.1-0.20190205005809-0d3efadf0154+incompatible
