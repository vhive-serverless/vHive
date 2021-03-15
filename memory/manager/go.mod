module github.com/ease-lab/vhive/memory/manager

go 1.15

// Workaround for github.com/containerd/containerd issue #3031
replace github.com/docker/distribution v2.7.1+incompatible => github.com/docker/distribution v2.7.1-0.20190205005809-0d3efadf0154+incompatible

replace github.com/ease-lab/vhive/metrics => ../../metrics

require (
	github.com/ease-lab/vhive/metrics v0.0.0-00010101000000-000000000000
	github.com/ftrvxmtrx/fd v0.0.0-20150925145434-c6d800382fff
	github.com/sirupsen/logrus v1.8.1
	golang.org/x/sys v0.0.0-20201201145000-ef89a241ccb3
	gonum.org/v1/gonum v0.8.2
)
