module github.com/ustiugov/fccd-orchestrator/misc

go 1.13

// Workaround for github.com/containerd/containerd issue #3031
replace github.com/docker/distribution v2.7.1+incompatible => github.com/docker/distribution v2.7.1-0.20190205005809-0d3efadf0154+incompatible

require (
	github.com/containerd/containerd v1.3.6
	github.com/containerd/continuity v0.0.0-20200928162600-f2cc35102c2a // indirect
	github.com/containerd/fifo v0.0.0-20201026212402-0724c46b320c // indirect
	github.com/containerd/ttrpc v1.0.2 // indirect
	github.com/containerd/typeurl v1.0.1 // indirect
	github.com/docker/distribution v2.7.1+incompatible // indirect
	github.com/docker/go-events v0.0.0-20190806004212-e31b211e4f1c // indirect
	github.com/gogo/googleapis v1.4.0 // indirect
	github.com/opencontainers/image-spec v1.0.1 // indirect
	github.com/opencontainers/runc v0.1.1 // indirect
	github.com/opencontainers/runtime-spec v1.0.2 // indirect
	github.com/sirupsen/logrus v1.6.0
	github.com/stretchr/testify v1.5.1
	github.com/syndtr/gocapability v0.0.0-20200815063812-42c35b437635 // indirect
	github.com/ustiugov/fccd-orchestrator/taps v0.0.0-20201120135927-438ab32ec4cf
	github.com/vishvananda/netlink v1.1.0
	golang.org/x/sync v0.0.0-20200625203802-6e8e738ad208
	google.golang.org/grpc v1.30.0
)
