module github.com/ustiugov/fccd-orchestrator/memory/manager

go 1.14

require (
	github.com/Microsoft/hcsshim v0.8.9 // indirect
	github.com/containerd/containerd v1.3.6
	github.com/containerd/continuity v0.0.0-20200710164510-efbc4488d8fe // indirect
	github.com/containerd/fifo v0.0.0-20200410184934-f15a3290365b // indirect
	github.com/containerd/ttrpc v1.0.1 // indirect
	github.com/containerd/typeurl v1.0.1 // indirect
	github.com/docker/distribution v2.7.1+incompatible // indirect
	github.com/docker/go-events v0.0.0-20190806004212-e31b211e4f1c // indirect
	github.com/ftrvxmtrx/fd v0.0.0-20150925145434-c6d800382fff
	github.com/gogo/googleapis v1.4.0 // indirect
	github.com/opencontainers/go-digest v1.0.0 // indirect
	github.com/opencontainers/image-spec v1.0.1 // indirect
	github.com/opencontainers/runc v0.1.1 // indirect
	github.com/opencontainers/runtime-spec v1.0.2 // indirect
	github.com/sirupsen/logrus v1.6.0
	github.com/stretchr/testify v1.6.1
	github.com/syndtr/gocapability v0.0.0-20180916011248-d98352740cb2 // indirect
	github.com/ustiugov/fccd-orchestrator/metrics v0.0.0-20200907081336-fae0d2f696c4
	golang.org/x/net v0.0.0-20200707034311-ab3426394381 // indirect
	golang.org/x/sync v0.0.0-20200625203802-6e8e738ad208 // indirect
	golang.org/x/sys v0.0.0-20200805065543-0cf7623e9dbd
	gonum.org/v1/gonum v0.7.0
	google.golang.org/grpc v1.31.0 // indirect
)

// Workaround for github.com/containerd/containerd issue #3031
replace github.com/docker/distribution v2.7.1+incompatible => github.com/docker/distribution v2.7.1-0.20190205005809-0d3efadf0154+incompatible
