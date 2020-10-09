module github.com/ustiugov/fccd-orchestrator/cri

go 1.14

replace k8s.io/api => k8s.io/api v0.16.6

replace k8s.io/apimachinery => k8s.io/apimachinery v0.16.6

replace k8s.io/apiserver => k8s.io/apiserver v0.16.6

replace k8s.io/client-go => k8s.io/client-go v0.16.6

replace k8s.io/cri-api => k8s.io/cri-api v0.16.6

replace k8s.io/klog => k8s.io/klog v1.0.0

replace k8s.io/kubernetes => k8s.io/kubernetes v1.16.6

require (
	github.com/containerd/containerd v1.3.6
	github.com/containerd/continuity v0.0.0-20190815185530-f2a389ac0a02
	github.com/containerd/cri v1.11.1-0.20200320165605-f864905c93b9
	github.com/containerd/fifo v0.0.0-20190816180239-bda0ff6ed73c
	github.com/containerd/ttrpc v1.0.0
	github.com/containerd/typeurl v1.0.0
	github.com/docker/distribution v2.7.1-0.20190205005809-0d3efadf0154+incompatible
	github.com/docker/docker v1.4.2-0.20171019062838-86f080cff091
	github.com/docker/go-events v0.0.0-20170721190031-9461782956ad // indirect
	github.com/gogo/googleapis v1.4.0 // indirect
	github.com/opencontainers/go-digest v1.0.0-rc1.0.20180430190053-c9281466c8b2
	github.com/opencontainers/image-spec v1.0.1
	github.com/opencontainers/runc v1.0.0-rc9 // indirect
	github.com/opencontainers/runtime-spec v1.0.2-0.20190207185410-29686dbc5559 // indirect
	github.com/pkg/errors v0.9.1
	github.com/sirupsen/logrus v1.7.0
	github.com/stretchr/testify v1.3.0
	github.com/syndtr/gocapability v0.0.0-20180916011248-d98352740cb2
	golang.org/x/net v0.0.0-20201009032441-dbdefad45b89 // indirect
	golang.org/x/sync v0.0.0-20201008141435-b3e1573b7520 // indirect
	golang.org/x/sys v0.0.0-20201009025420-dfb3f7c4e634 // indirect
	google.golang.org/grpc v1.33.0 // indirect
	k8s.io/cri-api v0.0.0-00010101000000-000000000000
)
