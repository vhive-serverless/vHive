module github.com/ustiugov/fccd-orchestrator

go 1.13

require (
	github.com/boltdb/bolt v1.3.1 // indirect
	github.com/containerd/containerd v1.3.3
	github.com/firecracker-microvm/firecracker-containerd v0.0.0-20200213214445-017fe9003d3f
	github.com/pkg/errors v0.9.1
	github.com/ustiugov/fccd-orchestrator/proto v0.0.0-20200225165909-e8cb22075cec
	github.com/ustiugov/skv v0.0.0-20180909015525-9def2caac4cc
	golang.org/x/net v0.0.0-20200222125558-5a598a2470a0 // indirect
	golang.org/x/sys v0.0.0-20200223170610-d5e6a3e2c0ae // indirect
	golang.org/x/text v0.3.2 // indirect
	google.golang.org/genproto v0.0.0-20200225123651-fc8f55426688 // indirect
	google.golang.org/grpc v1.27.1
)

// Workaround for github.com/containerd/containerd issue #3031
replace github.com/docker/distribution v2.7.1+incompatible => github.com/docker/distribution v2.7.1-0.20190205005809-0d3efadf0154+incompatible
