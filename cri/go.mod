module github.com/ustiugov/fccd-orchestrator/cri

go 1.14

require (
	github.com/BurntSushi/toml v0.3.1
	github.com/Microsoft/go-winio v0.4.14
	github.com/Microsoft/hcsshim v0.8.7-0.20190820203702-9e921883ac92
	github.com/beorn7/perks v0.0.0-20180321164747-3a771d992973
	github.com/containerd/cgroups v0.0.0-20200824123100-0b889c03f102
	github.com/containerd/console v0.0.0-20181022165439-0650fd9eeb50
	github.com/containerd/containerd v1.4.1
	github.com/containerd/continuity v0.0.0-20190815185530-f2a389ac0a02
	github.com/containerd/cri v1.19.0
	github.com/containerd/fifo v0.0.0-20190816180239-bda0ff6ed73c
	github.com/containerd/go-cni v0.0.0-20190813230227-49fbd9b210f3
	github.com/containerd/go-runc v0.0.0-20190911050354-e029b79d8cda
	github.com/containerd/imgcrypt v1.0.3 // indirect
	github.com/containerd/ttrpc v1.0.0
	github.com/containerd/typeurl v1.0.0
	github.com/containernetworking/cni v0.7.1
	github.com/containernetworking/plugins v0.7.6
	github.com/coreos/go-systemd v0.0.0-20181012123002-c6f51f82210d
	github.com/cpuguy83/go-md2man v1.0.10
	github.com/davecgh/go-spew v1.1.1
	github.com/docker/distribution v2.7.1-0.20190205005809-0d3efadf0154+incompatible
	github.com/docker/docker v1.4.2-0.20171019062838-86f080cff091
	github.com/docker/go-events v0.0.0-20170721190031-9461782956ad
	github.com/docker/go-metrics v0.0.0-20180131145841-4ea375f7759c
	github.com/docker/go-units v0.4.0
	github.com/docker/spdystream v0.0.0-20160310174837-449fdfce4d96
	github.com/emicklei/go-restful v2.9.5+incompatible
	github.com/godbus/dbus v0.0.0-20151105175453-c7fdd8b5cd55
	github.com/gogo/googleapis v1.2.0
	github.com/gogo/protobuf v1.3.1
	github.com/golang/protobuf v1.3.3
	github.com/google/gofuzz v1.0.0
	github.com/google/uuid v1.1.2
	github.com/grpc-ecosystem/go-grpc-prometheus v1.2.0
	github.com/hashicorp/golang-lru v0.5.3
	github.com/imdario/mergo v0.3.8
	github.com/json-iterator/go v1.1.8
	github.com/matttproud/golang_protobuf_extensions v1.0.1
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd
	github.com/modern-go/reflect2 v1.0.1
	github.com/opencontainers/go-digest v1.0.0
	github.com/opencontainers/image-spec v1.0.1
	github.com/opencontainers/runc v1.0.0-rc9
	github.com/opencontainers/runtime-spec v1.0.2
	github.com/opencontainers/selinux v1.2.2
	github.com/pkg/errors v0.9.1
	github.com/pmezard/go-difflib v1.0.0
	github.com/prometheus/client_golang v0.9.2
	github.com/prometheus/client_model v0.0.0-20190812154241-14fe0d1b01d4
	github.com/prometheus/common v0.0.0-20181126121408-4724e9255275
	github.com/prometheus/procfs v0.0.8
	github.com/russross/blackfriday v1.5.2
	github.com/seccomp/libseccomp-golang v0.9.1
	github.com/sirupsen/logrus v1.6.0
	github.com/stretchr/testify v1.4.0
	github.com/syndtr/gocapability v0.0.0-20180916011248-d98352740cb2
	github.com/tchap/go-patricia v2.2.6+incompatible
	github.com/urfave/cli v1.22.2
	go.etcd.io/bbolt v1.3.3
	go.opencensus.io v0.22.0
	golang.org/x/crypto v0.0.0-20190820162420-60c769a6c586
	golang.org/x/net v0.0.0-20191004110552-13f9640d40b9
	golang.org/x/oauth2 v0.0.0-20190604053449-0f29369cfe45
	golang.org/x/sync v0.0.0-20190911185100-cd5d95a43a6e
	golang.org/x/sys v0.0.0-20200124204421-9fbb57f87de9
	golang.org/x/text v0.3.2
	golang.org/x/time v0.0.0-20190308202827-9d24e82272b4
	google.golang.org/appengine v1.5.0
	google.golang.org/genproto v0.0.0-20190819201941-24fa4b261c55
	google.golang.org/grpc v1.33.0
	gopkg.in/inf.v0 v0.9.1
	gopkg.in/yaml.v2 v2.2.8
	k8s.io/apiserver v0.0.0-00010101000000-000000000000 // indirect
	k8s.io/cri-api v0.0.0-00010101000000-000000000000 // indirect
	k8s.io/klog/v2 v2.3.0 // indirect
	k8s.io/utils v0.0.0-20201015054608-420da100c033 // indirect
	sigs.k8s.io/yaml v1.1.0
)

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
