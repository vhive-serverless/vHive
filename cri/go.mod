module github.com/ease-lab/vhive/cri

go 1.15

// Workaround for github.com/containerd/containerd issue #3031
replace github.com/docker/distribution v2.7.1+incompatible => github.com/docker/distribution v2.7.1-0.20190205005809-0d3efadf0154+incompatible

replace (
	github.com/firecracker-microvm/firecracker-containerd => github.com/ease-lab/firecracker-containerd v0.0.0-20200804113524-bc259c9e8152
	github.com/firecracker-microvm/firecracker-go-sdk => github.com/ease-lab/firecracker-go-sdk v0.20.1-0.20200625102438-8edf287b0123
)

replace (
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
	github.com/ease-lab/vhive/ctriface => ../ctriface
	github.com/ease-lab/vhive/examples/protobuf/helloworld => ../examples/protobuf/helloworld
	github.com/ease-lab/vhive/memory/manager => ../memory/manager
	github.com/ease-lab/vhive/metrics => ../metrics
	github.com/ease-lab/vhive/misc => ../misc
	github.com/ease-lab/vhive/taps => ../taps
)

require (
	github.com/ease-lab/vhive/ctriface v0.0.0-00010101000000-000000000000
	github.com/sirupsen/logrus v1.7.0
	github.com/stretchr/testify v1.6.1
	golang.org/x/net v0.0.0-20200707034311-ab3426394381
	google.golang.org/grpc v1.33.1
	k8s.io/cri-api v0.0.0-00010101000000-000000000000
)
