module github.com/ustiugov/fccd-orchestrator/ctrIface

go 1.13

require (
	github.com/firecracker-microvm/firecracker-containerd v0.0.0-20200324214552-7383119704ec // indirect
	github.com/ustiugov/fccd-orchestrator/misc v0.0.0-20200325205406-9d4c3c735efc // indirect
)

// Workaround for github.com/containerd/containerd issue #3031
replace github.com/docker/distribution v2.7.1+incompatible => github.com/docker/distribution v2.7.1-0.20190205005809-0d3efadf0154+incompatible
