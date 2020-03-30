module github.com/ustiugov/fccd-orchestrator

go 1.13

require (
	github.com/Microsoft/hcsshim v0.8.7 // indirect
	github.com/containerd/cgroups v0.0.0-20200327175542-b44481373989 // indirect
	github.com/containerd/containerd v1.3.3
	github.com/firecracker-microvm/firecracker-containerd v0.0.0-20200324214552-7383119704ec
	github.com/golang/groupcache v0.0.0-20200121045136-8c9f03a8e57e // indirect
	github.com/golang/protobuf v1.3.5
	github.com/hashicorp/golang-lru v0.5.4 // indirect
	github.com/opencontainers/runtime-spec v1.0.2 // indirect
	github.com/pkg/errors v0.9.1
	github.com/ustiugov/fccd-orchestrator/ctrIface v0.0.0-20200330172127-0d359e57cf32
	github.com/ustiugov/fccd-orchestrator/misc v0.0.0-20200330172127-0d359e57cf32
	github.com/ustiugov/fccd-orchestrator/proto v0.0.0-20200330172127-0d359e57cf32
	github.com/ustiugov/skv v0.0.0-20180909015525-9def2caac4cc
	go.opencensus.io v0.22.3 // indirect
	golang.org/x/sys v0.0.0-20200327173247-9dae0f8f5775 // indirect
	google.golang.org/genproto v0.0.0-20200330113809-af700f360a68 // indirect
	google.golang.org/grpc v1.28.0
)

// Workaround for github.com/containerd/containerd issue #3031
replace github.com/docker/distribution v2.7.1+incompatible => github.com/docker/distribution v2.7.1-0.20190205005809-0d3efadf0154+incompatible
