module github.com/ustiugov/fccd-orchestrator/ctriface

go 1.13

replace github.com/firecracker-microvm/firecracker-containerd => github.com/ustiugov/firecracker-containerd v0.0.0-20200721162250-8583561e65e2

replace github.com/firecracker-microvm/firecracker-go-sdk => github.com/ustiugov/firecracker-go-sdk v0.20.1-0.20200625102438-8edf287b0123

require (
	github.com/containerd/containerd v1.3.6
	github.com/davecgh/go-spew v1.1.1
	github.com/firecracker-microvm/firecracker-containerd v0.0.0-00010101000000-000000000000
	github.com/pkg/errors v0.9.1
	github.com/sirupsen/logrus v1.6.0
	github.com/stretchr/testify v1.6.1
	github.com/ustiugov/fccd-orchestrator/helloworld v0.0.0-20200717125634-528c6e9f9cc9
	github.com/ustiugov/fccd-orchestrator/metrics v0.0.0-20200722141002-55dbdeb43861
	github.com/ustiugov/fccd-orchestrator/misc v0.0.0-20200717125634-528c6e9f9cc9
	github.com/ustiugov/fccd-orchestrator/taps v0.0.0-20200717125634-528c6e9f9cc9
	golang.org/x/tools v0.0.0-20200721223218-6123e77877b2 // indirect
	google.golang.org/grpc v1.30.0

)

// Workaround for github.com/containerd/containerd issue #3031
replace github.com/docker/distribution v2.7.1+incompatible => github.com/docker/distribution v2.7.1-0.20190205005809-0d3efadf0154+incompatible
