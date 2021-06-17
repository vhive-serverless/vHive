module github.com/ease-lab/vhive

go 1.15

// Copied from firecracker-containerd
replace (
	// Pin gPRC-related dependencies as like containerd v1.5.2
	github.com/gogo/googleapis => github.com/gogo/googleapis v1.3.2
	github.com/golang/protobuf => github.com/golang/protobuf v1.3.5
	google.golang.org/genproto => google.golang.org/genproto v0.0.0-20200224152610-e50cd9704f63
	google.golang.org/grpc => google.golang.org/grpc v1.27.1
)

replace (
	github.com/coreos/go-systemd => github.com/coreos/go-systemd v0.0.0-20161114122254-48702e0da86b
	k8s.io/api => k8s.io/api v0.16.6
	k8s.io/apiextensions-apiserver => k8s.io/apiextensions-apiserver v0.16.6
	k8s.io/apimachinery => k8s.io/apimachinery v0.16.7-beta.0
	k8s.io/apiserver => k8s.io/apiserver v0.16.6
	k8s.io/cli-runtime => k8s.io/cli-runtime v0.16.6
	k8s.io/client-go => k8s.io/client-go v0.16.6
	k8s.io/cloud-provider => k8s.io/cloud-provider v0.16.6
	k8s.io/cluster-bootstrap => k8s.io/cluster-bootstrap v0.16.6
	k8s.io/code-generator => k8s.io/code-generator v0.16.7-beta.0
	k8s.io/component-base => k8s.io/component-base v0.16.6
	k8s.io/cri-api => k8s.io/cri-api v0.16.16-rc.0
	k8s.io/csi-translation-lib => k8s.io/csi-translation-lib v0.16.6
	k8s.io/kube-aggregator => k8s.io/kube-aggregator v0.16.6
	k8s.io/kube-controller-manager => k8s.io/kube-controller-manager v0.16.6
	k8s.io/kube-proxy => k8s.io/kube-proxy v0.16.6
	k8s.io/kube-scheduler => k8s.io/kube-scheduler v0.16.6
	k8s.io/kubectl => k8s.io/kubectl v0.16.6
	k8s.io/kubelet => k8s.io/kubelet v0.16.6
	k8s.io/kubernetes => k8s.io/kubernetes v1.16.6
	k8s.io/legacy-cloud-providers => k8s.io/legacy-cloud-providers v0.16.6
	k8s.io/metrics => k8s.io/metrics v0.16.6
	k8s.io/node-api => k8s.io/node-api v0.16.6
	k8s.io/sample-apiserver => k8s.io/sample-apiserver v0.16.6
	k8s.io/sample-cli-plugin => k8s.io/sample-cli-plugin v0.16.6
	k8s.io/sample-controller => k8s.io/sample-controller v0.16.6
)

replace (
	github.com/ease-lab/vhive/examples/protobuf/helloworld => ./examples/protobuf/helloworld
	github.com/ease-lab/vhive/utils/tracing/go => ./utils/tracing/go
	github.com/firecracker-microvm/firecracker-containerd => github.com/ease-lab/firecracker-containerd v0.0.0-20210618165033-6af02db30bc4
)

require (
	github.com/blend/go-sdk v1.20210616.2 // indirect
	github.com/containerd/containerd v1.5.2
	github.com/davecgh/go-spew v1.1.1
	github.com/ease-lab/vhive/examples/protobuf/helloworld v0.0.0-00010101000000-000000000000
	github.com/ease-lab/vhive/utils/tracing/go v0.0.0-00010101000000-000000000000
	github.com/firecracker-microvm/firecracker-containerd v0.0.0-00010101000000-000000000000
	github.com/ftrvxmtrx/fd v0.0.0-20150925145434-c6d800382fff
	github.com/go-multierror/multierror v1.0.2
	github.com/golang/protobuf v1.5.2
	github.com/montanaflynn/stats v0.6.5
	github.com/pkg/errors v0.9.1
	github.com/sirupsen/logrus v1.8.0
	github.com/stretchr/testify v1.7.0
	github.com/vishvananda/netlink v1.1.1-0.20201029203352-d40f9887b852
	github.com/wcharczuk/go-chart v2.0.1+incompatible
	go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc v0.20.0
	golang.org/x/image v0.0.0-20210220032944-ac19c3e999fb // indirect
	golang.org/x/net v0.0.0-20210226172049-e18ecbb05110
	golang.org/x/sync v0.0.0-20201207232520-09787c993a3a
	golang.org/x/sys v0.0.0-20210324051608-47abb6519492
	gonum.org/v1/gonum v0.9.0
	gonum.org/v1/plot v0.9.0
	google.golang.org/grpc v1.37.0
	k8s.io/cri-api v0.20.6
)
