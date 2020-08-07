module github.com/ustiugov/fccd-orchestrator

go 1.13

replace github.com/firecracker-microvm/firecracker-containerd => github.com/ustiugov/firecracker-containerd v0.0.0-20200804113524-bc259c9e8152

replace github.com/firecracker-microvm/firecracker-go-sdk => github.com/ustiugov/firecracker-go-sdk v0.20.1-0.20200625102438-8edf287b0123

require (
	github.com/containerd/containerd v1.3.6
	github.com/sirupsen/logrus v1.6.0
	github.com/stretchr/testify v1.6.1
	github.com/ustiugov/fccd-orchestrator/ctriface v0.0.0-20200807221311-73620bc9b567
	github.com/ustiugov/fccd-orchestrator/helloworld v0.0.0-20200803195925-0629e1cf4599
	github.com/ustiugov/fccd-orchestrator/metrics v0.0.0-20200803195925-0629e1cf4599
	github.com/ustiugov/fccd-orchestrator/misc v0.0.0-20200803195925-0629e1cf4599 // indirect
	github.com/ustiugov/fccd-orchestrator/proto v0.0.0-20200803195925-0629e1cf4599
	golang.org/x/sync v0.0.0-20200625203802-6e8e738ad208
	google.golang.org/grpc v1.31.0
)

// Workaround for github.com/containerd/containerd issue #3031
replace github.com/docker/distribution v2.7.1+incompatible => github.com/docker/distribution v2.7.1-0.20190205005809-0d3efadf0154+incompatible
