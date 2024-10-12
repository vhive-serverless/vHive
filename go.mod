module github.com/vhive-serverless/vhive

go 1.21

// Copied from firecracker-containerd
replace (
	// Pin gPRC-related dependencies as like containerd v1.6.20
	google.golang.org/genproto => google.golang.org/genproto v0.0.0-20200224152610-e50cd9704f63
	google.golang.org/grpc => google.golang.org/grpc v1.38.1
)

replace github.com/coreos/go-systemd => github.com/coreos/go-systemd v0.0.0-20161114122254-48702e0da86b

replace (
	github.com/firecracker-microvm/firecracker-containerd => github.com/vhive-serverless/firecracker-containerd v0.0.0-20230912063208-ad6383f05e45
	github.com/vhive-serverless/vhive/examples/protobuf/helloworld => ./examples/protobuf/helloworld
)

require (
	github.com/containerd/containerd v1.6.20
	github.com/containerd/go-cni v1.1.6
	github.com/davecgh/go-spew v1.1.1
	github.com/firecracker-microvm/firecracker-containerd v0.0.0-00010101000000-000000000000
	github.com/ftrvxmtrx/fd v0.0.0-20150925145434-c6d800382fff
	github.com/go-multierror/multierror v1.0.2
	github.com/golang/protobuf v1.5.3
	github.com/google/nftables v0.2.0
	github.com/google/uuid v1.6.0
	github.com/montanaflynn/stats v0.7.1
	github.com/opencontainers/image-spec v1.1.0
	github.com/pkg/errors v0.9.1
	github.com/sirupsen/logrus v1.9.0
	github.com/stretchr/testify v1.9.0
	github.com/vhive-serverless/vhive/examples/protobuf/helloworld v0.0.0-00010101000000-000000000000
	github.com/vishvananda/netlink v1.3.0
	github.com/vishvananda/netns v0.0.4
	github.com/wcharczuk/go-chart v2.0.1+incompatible
	golang.org/x/net v0.30.0
	golang.org/x/sync v0.8.0
	golang.org/x/sys v0.26.0
	gonum.org/v1/gonum v0.15.0
	gonum.org/v1/plot v0.14.0
	google.golang.org/grpc v1.47.0
	k8s.io/cri-api v0.25.0
)

require (
	git.sr.ht/~sbinet/gg v0.5.0 // indirect
	github.com/Microsoft/go-winio v0.5.2 // indirect
	github.com/Microsoft/hcsshim v0.9.8 // indirect
	github.com/ajstarks/svgo v0.0.0-20211024235047-1546f124cd8b // indirect
	github.com/blend/go-sdk v1.1.1 // indirect
	github.com/campoy/embedmd v1.0.0 // indirect
	github.com/containerd/cgroups v1.0.4 // indirect
	github.com/containerd/continuity v0.3.0 // indirect
	github.com/containerd/fifo v1.1.0 // indirect
	github.com/containerd/ttrpc v1.1.2 // indirect
	github.com/containerd/typeurl v1.0.2 // indirect
	github.com/containernetworking/cni v1.1.2 // indirect
	github.com/docker/go-events v0.0.0-20190806004212-e31b211e4f1c // indirect
	github.com/go-fonts/liberation v0.3.2 // indirect
	github.com/go-latex/latex v0.0.0-20231108140139-5c1ce85aa4ea // indirect
	github.com/go-pdf/fpdf v0.9.0 // indirect
	github.com/gogo/googleapis v1.4.1 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang/freetype v0.0.0-20170609003504-e2365dfdc4a0 // indirect
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da // indirect
	github.com/google/go-cmp v0.6.0 // indirect
	github.com/josharian/native v1.1.0 // indirect
	github.com/klauspost/compress v1.15.6 // indirect
	github.com/mdlayher/netlink v1.7.2 // indirect
	github.com/mdlayher/socket v0.5.0 // indirect
	github.com/moby/locker v1.0.1 // indirect
	github.com/moby/sys/mountinfo v0.6.2 // indirect
	github.com/moby/sys/signal v0.7.0 // indirect
	github.com/opencontainers/go-digest v1.0.0 // indirect
	github.com/opencontainers/runc v1.1.14 // indirect
	github.com/opencontainers/runtime-spec v1.0.3-0.20210910115017-0d6cc581aeea // indirect
	github.com/opencontainers/selinux v1.10.1 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	go.opencensus.io v0.23.0 // indirect
	golang.org/x/image v0.18.0 // indirect
	golang.org/x/text v0.19.0 // indirect
	google.golang.org/genproto v0.0.0-20220617124728-180714bec0ad // indirect
	google.golang.org/protobuf v1.33.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
