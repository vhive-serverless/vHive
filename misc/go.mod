module github.com/ustiugov/fccd-orchestrator/misc

go 1.13

// Workaround for github.com/containerd/containerd issue #3031
replace github.com/docker/distribution v2.7.1+incompatible => github.com/docker/distribution v2.7.1-0.20190205005809-0d3efadf0154+incompatible

require (
	github.com/Microsoft/hcsshim v0.8.9 // indirect
	github.com/containerd/containerd v1.3.6
	github.com/containerd/continuity v0.0.0-20200709052629-daa8e1ccc0bc // indirect
	github.com/containerd/ttrpc v1.0.1 // indirect
	github.com/containerd/typeurl v1.0.1 // indirect
	github.com/docker/distribution v2.7.1+incompatible // indirect
	github.com/docker/go-events v0.0.0-20190806004212-e31b211e4f1c // indirect
	github.com/gogo/googleapis v1.4.0 // indirect
	github.com/opencontainers/go-digest v1.0.0 // indirect
	github.com/opencontainers/image-spec v1.0.1 // indirect
	github.com/sirupsen/logrus v1.6.0
	github.com/stretchr/testify v1.5.1
	github.com/syndtr/gocapability v0.0.0-20180916011248-d98352740cb2 // indirect
	github.com/ustiugov/fccd-orchestrator/helloworld v0.0.0-20200710161907-f76633b53689
	github.com/ustiugov/fccd-orchestrator/taps v0.0.0-20200714121618-b03bf5c0e06e
	github.com/vishvananda/netlink v1.1.0
	golang.org/x/sync v0.0.0-20200625203802-6e8e738ad208
	google.golang.org/grpc v1.30.0
)
