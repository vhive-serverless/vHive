module github.com/ustiugov/fccd-orchestrator

go 1.13

require (
	github.com/boltdb/bolt v1.3.1 // indirect
	github.com/containerd/containerd v1.3.3
	github.com/firecracker-microvm/firecracker-containerd v0.0.0-20200213214445-017fe9003d3f
	github.com/pkg/errors v0.9.1
	github.com/ustiugov/fccd-orchestrator/proto v0.0.0-20200216190957-50b07037dc47
	github.com/ustiugov/skv v0.0.0-20180909015525-9def2caac4cc
	google.golang.org/grpc v1.27.1
)

// Workaround for github.com/containerd/containerd issue #3031
replace github.com/docker/distribution v2.7.1+incompatible => github.com/docker/distribution v2.7.1-0.20190205005809-0d3efadf0154+incompatible
