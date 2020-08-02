module github.com/ustiugov/fccd-orchestrator

go 1.13

// Workaround for github.com/containerd/containerd issue #3031
replace github.com/docker/distribution v2.7.1+incompatible => github.com/docker/distribution v2.7.1-0.20190205005809-0d3efadf0154+incompatible

replace github.com/firecracker-microvm/firecracker-containerd => github.com/ustiugov/firecracker-containerd v0.0.0-20200721162250-8583561e65e2

replace github.com/firecracker-microvm/firecracker-go-sdk => github.com/ustiugov/firecracker-go-sdk v0.20.1-0.20200625102438-8edf287b0123

require (
	github.com/containerd/containerd v1.3.6
	github.com/golang/protobuf v1.3.3
	github.com/sirupsen/logrus v1.6.0
	github.com/stretchr/testify v1.6.1
	github.com/ustiugov/fccd-orchestrator/ctriface v0.0.0-20200802194423-0ff4cd255633
	github.com/ustiugov/fccd-orchestrator/helloworld v0.0.0-20200717125634-528c6e9f9cc9
	github.com/ustiugov/fccd-orchestrator/metrics v0.0.0-20200802194423-0ff4cd255633
	github.com/ustiugov/fccd-orchestrator/misc v0.0.0-20200717125634-528c6e9f9cc9 // indirect
	github.com/ustiugov/fccd-orchestrator/proto v0.0.0-20200717125634-528c6e9f9cc9
	github.com/ustiugov/fccd-orchestrator/taps v0.0.0-20200717125634-528c6e9f9cc9 // indirect
	golang.org/x/sync v0.0.0-20200625203802-6e8e738ad208
	google.golang.org/grpc v1.30.0
)
