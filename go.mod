module github.com/ustiugov/fccd-orchestrator

go 1.13

// Workaround for github.com/containerd/containerd issue #3031
replace github.com/docker/distribution v2.7.1+incompatible => github.com/docker/distribution v2.7.1-0.20190205005809-0d3efadf0154+incompatible

replace github.com/firecracker-microvm/firecracker-containerd => github.com/ustiugov/firecracker-containerd v0.0.0-20200710135524-67aece3e7f88

replace github.com/firecracker-microvm/firecracker-go-sdk => github.com/ustiugov/firecracker-go-sdk v0.20.1-0.20200625102438-8edf287b0123

require (
	github.com/containerd/containerd v1.3.6
	github.com/golang/protobuf v1.3.3
	github.com/sirupsen/logrus v1.6.0
	github.com/stretchr/testify v1.5.1
	github.com/ustiugov/fccd-orchestrator/ctriface v0.0.0-20200714173637-bc269d5f4288
	github.com/ustiugov/fccd-orchestrator/helloworld v0.0.0-20200714162243-d6dc0c083e9e
	github.com/ustiugov/fccd-orchestrator/misc v0.0.0-20200714162243-d6dc0c083e9e // indirect
	github.com/ustiugov/fccd-orchestrator/proto v0.0.0-20200714162243-d6dc0c083e9e
	github.com/ustiugov/fccd-orchestrator/taps v0.0.0-20200714162243-d6dc0c083e9e // indirect
	golang.org/x/sync v0.0.0-20200625203802-6e8e738ad208
	google.golang.org/grpc v1.30.0
)
