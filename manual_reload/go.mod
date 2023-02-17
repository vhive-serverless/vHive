module github.com/amohoste/firecracker-containerd-example

go 1.13

replace (
	// Pin gPRC-related dependencies as like containerd v1.5.2
	github.com/gogo/googleapis => github.com/gogo/googleapis v1.3.2
	github.com/golang/protobuf => github.com/golang/protobuf v1.3.5
	google.golang.org/genproto => google.golang.org/genproto v0.0.0-20200224152610-e50cd9704f63
	google.golang.org/grpc => google.golang.org/grpc v1.27.1
)

replace (
	github.com/containerd/containerd => github.com/amohoste/containerd v1.5.5-ids
	github.com/firecracker-microvm/firecracker-containerd => github.com/amohoste/firecracker-containerd v1.0.0-ids
)

require (
	github.com/antchfx/xpath v1.2.0 // indirect
	github.com/containerd/containerd v1.5.2
	github.com/coreos/go-iptables v0.5.0
	github.com/firecracker-microvm/firecracker-containerd v0.0.0-20210604232636-c2323bc71886
	github.com/opencontainers/image-spec v1.0.1
	github.com/pkg/errors v0.9.1
	github.com/tamerh/xml-stream-parser v1.4.0
	github.com/tamerh/xpath v1.0.0 // indirect
	github.com/vishvananda/netlink v1.1.1-0.20201029203352-d40f9887b852
	github.com/vishvananda/netns v0.0.0-20200728191858-db3c7e526aae
)
