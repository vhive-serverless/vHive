module github.com/ease-lab/vhive/misc

go 1.15

// Workaround for github.com/containerd/containerd issue #3031
replace github.com/docker/distribution v2.7.1+incompatible => github.com/docker/distribution v2.7.1-0.20190205005809-0d3efadf0154+incompatible

replace github.com/ease-lab/vhive/taps => ../taps

require (
	github.com/containerd/containerd v1.5.2
	github.com/ease-lab/vhive/taps v0.0.0-00010101000000-000000000000
	github.com/sirupsen/logrus v1.8.0
	github.com/stretchr/testify v1.7.0

)
