module github.com/ustiugov/fccd-orchestrator/ctriface

go 1.14

require (
	github.com/ustiugov/fccd-orchestrator v0.0.0-20200416161510-f6f32fa5d52b // indirect
	github.com/ustiugov/fccd-orchestrator/misc v0.0.0-20200416161510-f6f32fa5d52b // indirect
)

// Workaround for github.com/containerd/containerd issue #3031
replace github.com/docker/distribution v2.7.1+incompatible => github.com/docker/distribution v2.7.1-0.20190205005809-0d3efadf0154+incompatible
