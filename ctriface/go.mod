module github.com/ustiugov/fccd-orchestrator/ctriface

go 1.13

replace github.com/firecracker-microvm/firecracker-containerd => github.com/ustiugov/firecracker-containerd v0.0.0-20200804113524-bc259c9e8152

replace github.com/firecracker-microvm/firecracker-go-sdk => github.com/ustiugov/firecracker-go-sdk v0.20.1-0.20200625102438-8edf287b0123

require (
	github.com/containerd/containerd v1.3.6
	github.com/davecgh/go-spew v1.1.1
	github.com/firecracker-microvm/firecracker-containerd v0.0.0-00010101000000-000000000000
	github.com/go-multierror/multierror v1.0.2
	github.com/pkg/errors v0.9.1
	github.com/sirupsen/logrus v1.6.0
	github.com/stretchr/testify v1.6.1
	github.com/ustiugov/fccd-orchestrator/helloworld v0.0.0-20200717125634-528c6e9f9cc9
	github.com/ustiugov/fccd-orchestrator/memory/manager v0.0.0-20200807223246-24cb76768b84
	github.com/ustiugov/fccd-orchestrator/metrics v0.0.0-20200803195925-0629e1cf4599
	github.com/ustiugov/fccd-orchestrator/misc v0.0.0-20200803195925-0629e1cf4599
	github.com/ustiugov/fccd-orchestrator/taps v0.0.0-20200717125634-528c6e9f9cc9
	google.golang.org/grpc v1.31.0

)

// Workaround for github.com/containerd/containerd issue #3031
replace github.com/docker/distribution v2.7.1+incompatible => github.com/docker/distribution v2.7.1-0.20190205005809-0d3efadf0154+incompatible
