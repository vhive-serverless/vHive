module github.com/ease-lab/vhive/misc

go 1.15

// Workaround for github.com/containerd/containerd issue #3031
replace github.com/docker/distribution v2.7.1+incompatible => github.com/docker/distribution v2.7.1-0.20190205005809-0d3efadf0154+incompatible

replace github.com/ease-lab/vhive/taps => ../taps

require (
	github.com/Microsoft/hcsshim/test v0.0.0-20210308065211-081ab2f5da53 // indirect
	github.com/containerd/cgroups v0.0.0-20210114181951-8a68de567b68 // indirect
	github.com/containerd/containerd v1.5.0-beta.1
	github.com/ease-lab/vhive/taps v0.0.0-00010101000000-000000000000
	github.com/google/uuid v1.2.0 // indirect
	github.com/imdario/mergo v0.3.11 // indirect
	github.com/sirupsen/logrus v1.8.0
	github.com/stretchr/testify v1.7.0
	gotest.tools/v3 v3.0.3 // indirect

)
