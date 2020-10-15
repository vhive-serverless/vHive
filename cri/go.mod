module github.com/ustiugov/fccd-orchestrator/cri

go 1.14

require (
	github.com/BurntSushi/toml v0.3.1
	github.com/Microsoft/hcsshim v0.8.7-0.20190820203702-9e921883ac92 // indirect
	github.com/containerd/cgroups v0.0.0-20190717030353-c4b9ac5c7601
	github.com/containerd/console v0.0.0-20181022165439-0650fd9eeb50 // indirect
	github.com/containerd/containerd v1.3.2
	github.com/containerd/continuity v0.0.0-20190815185530-f2a389ac0a02
	github.com/containerd/cri v0.0.0-00010101000000-000000000000
	github.com/containerd/fifo v0.0.0-20190816180239-bda0ff6ed73c
	github.com/containerd/go-cni v0.0.0-20190813230227-49fbd9b210f3
	github.com/containerd/go-runc v0.0.0-20190911050354-e029b79d8cda // indirect
	github.com/containerd/ttrpc v1.0.0 // indirect
	github.com/containerd/typeurl v1.0.0
	github.com/containernetworking/plugins v0.7.6
	github.com/davecgh/go-spew v1.1.1
	github.com/docker/distribution v2.7.1+incompatible
	github.com/docker/docker v1.4.2-0.20171019062838-86f080cff091
	github.com/docker/go-events v0.0.0-20170721190031-9461782956ad // indirect
	github.com/docker/go-metrics v0.0.0-20180131145841-4ea375f7759c // indirect
	github.com/docker/go-units v0.4.0 // indirect
	github.com/gogo/googleapis v1.2.0 // indirect
	github.com/gogo/protobuf v1.2.2-0.20190723190241-65acae22fc9d
	github.com/golang/protobuf v1.3.1
	github.com/hashicorp/golang-lru v0.5.3 // indirect
	github.com/imdario/mergo v0.3.7 // indirect
	github.com/json-iterator/go v1.1.8 // indirect
	github.com/opencontainers/go-digest v1.0.0-rc1.0.20180430190053-c9281466c8b2
	github.com/opencontainers/image-spec v1.0.1
	github.com/opencontainers/runc v1.0.0-rc9
	github.com/opencontainers/runtime-spec v1.0.2-0.20190207185410-29686dbc5559
	github.com/opencontainers/selinux v1.2.2
	github.com/pkg/errors v0.8.1
	github.com/sirupsen/logrus v1.4.2
	github.com/stretchr/testify v1.4.0
	github.com/syndtr/gocapability v0.0.0-20180916011248-d98352740cb2 // indirect
	github.com/tchap/go-patricia v2.2.6+incompatible // indirect
	github.com/urfave/cli v1.22.0 // indirect
	golang.org/x/net v0.0.0-20191004110552-13f9640d40b9
	golang.org/x/sys v0.0.0-20190813064441-fde4db37ae7a
	google.golang.org/grpc v1.23.0
	gopkg.in/yaml.v2 v2.2.8 // indirect
	k8s.io/apimachinery v0.16.7-beta.0
	k8s.io/client-go v0.16.6
	k8s.io/cri-api v0.16.16-rc.0
	k8s.io/klog v1.0.0
	k8s.io/kubernetes v1.16.6
	k8s.io/utils v0.0.0-20191114184206-e782cd3c129f
)

replace github.com/docker/distribution => github.com/docker/distribution v2.7.1-0.20190104202606-0ac367fd6bee+incompatible

replace k8s.io/api => k8s.io/api v0.16.6

replace k8s.io/apiextensions-apiserver => k8s.io/apiextensions-apiserver v0.16.6

replace k8s.io/apimachinery => k8s.io/apimachinery v0.16.7-beta.0

replace k8s.io/apiserver => k8s.io/apiserver v0.16.6

replace k8s.io/cli-runtime => k8s.io/cli-runtime v0.16.6

replace k8s.io/client-go => k8s.io/client-go v0.16.6

replace k8s.io/cloud-provider => k8s.io/cloud-provider v0.16.6

replace k8s.io/cluster-bootstrap => k8s.io/cluster-bootstrap v0.16.6

replace k8s.io/code-generator => k8s.io/code-generator v0.16.7-beta.0

replace k8s.io/component-base => k8s.io/component-base v0.16.6

replace k8s.io/cri-api => k8s.io/cri-api v0.16.16-rc.0

replace k8s.io/csi-translation-lib => k8s.io/csi-translation-lib v0.16.6

replace k8s.io/kube-aggregator => k8s.io/kube-aggregator v0.16.6

replace k8s.io/kube-controller-manager => k8s.io/kube-controller-manager v0.16.6

replace k8s.io/kube-proxy => k8s.io/kube-proxy v0.16.6

replace k8s.io/kube-scheduler => k8s.io/kube-scheduler v0.16.6

replace k8s.io/kubectl => k8s.io/kubectl v0.16.6

replace k8s.io/kubelet => k8s.io/kubelet v0.16.6

replace k8s.io/legacy-cloud-providers => k8s.io/legacy-cloud-providers v0.16.6

replace k8s.io/metrics => k8s.io/metrics v0.16.6

replace k8s.io/node-api => k8s.io/node-api v0.16.6

replace k8s.io/sample-apiserver => k8s.io/sample-apiserver v0.16.6

replace k8s.io/sample-cli-plugin => k8s.io/sample-cli-plugin v0.16.6

replace k8s.io/sample-controller => k8s.io/sample-controller v0.16.6

replace github.com/containerd/cri => github.com/plamenmpetrov/cri v1.11.1-0.20200320165605-f864905c93b9
