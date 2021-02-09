module github.com/ease-lab/vhive/misc

go 1.15

// Workaround for github.com/containerd/containerd issue #3031
replace github.com/docker/distribution v2.7.1+incompatible => github.com/docker/distribution v2.7.1-0.20190205005809-0d3efadf0154+incompatible

replace github.com/ease-lab/vhive/taps => ../taps

require (
	github.com/Microsoft/go-winio v0.4.16 // indirect
	github.com/Microsoft/hcsshim v0.8.14 // indirect
	github.com/containerd/cgroups v0.0.0-20210114181951-8a68de567b68 // indirect
	github.com/containerd/containerd v1.3.6
	github.com/containerd/continuity v0.0.0-20201119173150-04c754faca46 // indirect
	github.com/containerd/fifo v0.0.0-20201026212402-0724c46b320c // indirect
	github.com/containerd/ttrpc v1.0.2 // indirect
	github.com/containerd/typeurl v1.0.1 // indirect
	github.com/docker/distribution v2.7.1+incompatible // indirect
	github.com/docker/go-events v0.0.0-20190806004212-e31b211e4f1c // indirect
	github.com/ease-lab/vhive/taps v0.0.0-20201130160304-f57486a97db0
	github.com/gogo/googleapis v1.4.0 // indirect
	github.com/imdario/mergo v0.3.11 // indirect
	github.com/opencontainers/image-spec v1.0.1 // indirect
	github.com/opencontainers/runc v0.1.1 // indirect
	github.com/sirupsen/logrus v1.7.0
	github.com/stretchr/testify v1.5.1
	github.com/syndtr/gocapability v0.0.0-20200815063812-42c35b437635 // indirect
	go.etcd.io/bbolt v1.3.5 // indirect

)
