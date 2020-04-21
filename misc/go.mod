module github.com/ustiugov/fccd-orchestrator/misc

go 1.13

// Workaround for github.com/containerd/containerd issue #3031
replace github.com/docker/distribution v2.7.1+incompatible => github.com/docker/distribution v2.7.1-0.20190205005809-0d3efadf0154+incompatible

require (
	github.com/Microsoft/hcsshim v0.8.7 // indirect
	github.com/census-instrumentation/opencensus-proto v0.2.1 // indirect
	github.com/containerd/containerd v1.3.3
	github.com/containerd/continuity v0.0.0-20200413184840-d3ef23f19fbb // indirect
	github.com/containerd/fifo v0.0.0-20200410184934-f15a3290365b // indirect
	github.com/containerd/ttrpc v1.0.0 // indirect
	github.com/containerd/typeurl v1.0.0 // indirect
	github.com/docker/go-events v0.0.0-20190806004212-e31b211e4f1c // indirect
	github.com/envoyproxy/go-control-plane v0.8.6 // indirect
	github.com/envoyproxy/protoc-gen-validate v0.1.0 // indirect
	github.com/firecracker-microvm/firecracker-containerd v0.0.0-20200331220105-afedbc74f5ee // indirect
	github.com/gogo/googleapis v1.3.2 // indirect
	github.com/google/go-cmp v0.4.0 // indirect
	github.com/opencontainers/runtime-spec v1.0.2 // indirect
	github.com/pkg/errors v0.9.1
	github.com/prometheus/client_model v0.0.0-20190812154241-14fe0d1b01d4 // indirect
	github.com/sirupsen/logrus v1.5.0
	google.golang.org/grpc v1.21.0
	honnef.co/go/tools v0.0.0-20190523083050-ea95bdfd59fc // indirect
)
