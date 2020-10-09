module github.com/ustiugov/fccd-orchestrator

go 1.13

replace github.com/firecracker-microvm/firecracker-containerd => github.com/ustiugov/firecracker-containerd v0.0.0-20200804113524-bc259c9e8152

replace github.com/firecracker-microvm/firecracker-go-sdk => github.com/ustiugov/firecracker-go-sdk v0.20.1-0.20200625102438-8edf287b0123

replace k8s.io/api => k8s.io/api v0.16.6

replace k8s.io/apimachinery => k8s.io/apimachinery v0.16.6

replace k8s.io/apiserver => k8s.io/apiserver v0.16.6

replace k8s.io/client-go => k8s.io/client-go v0.16.6

replace k8s.io/cri-api => k8s.io/cri-api v0.16.6

replace k8s.io/klog => k8s.io/klog v1.0.0

replace k8s.io/kubernetes => k8s.io/kubernetes v1.16.6

require (
	github.com/containerd/containerd v1.3.6
	github.com/sirupsen/logrus v1.7.0
	github.com/stretchr/testify v1.6.1
	github.com/ustiugov/fccd-orchestrator/cri v0.0.0-20201009133152-4cbafaef80d5
	github.com/ustiugov/fccd-orchestrator/ctriface v0.0.0-20201009133152-4cbafaef80d5
	github.com/ustiugov/fccd-orchestrator/helloworld v0.0.0-20200803195925-0629e1cf4599
	github.com/ustiugov/fccd-orchestrator/metrics v0.0.0-20200907081336-fae0d2f696c4
	github.com/ustiugov/fccd-orchestrator/proto v0.0.0-20200803195925-0629e1cf4599
	golang.org/x/sync v0.0.0-20201008141435-b3e1573b7520
	google.golang.org/grpc v1.33.0
)

// Workaround for github.com/containerd/containerd issue #3031
replace github.com/docker/distribution v2.7.1+incompatible => github.com/docker/distribution v2.7.1-0.20190205005809-0d3efadf0154+incompatible
