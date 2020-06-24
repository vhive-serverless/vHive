module github.com/ustiugov/fccd-orchestrator

go 1.13

// Workaround for github.com/containerd/containerd issue #3031
replace github.com/docker/distribution v2.7.1+incompatible => github.com/docker/distribution v2.7.1-0.20190205005809-0d3efadf0154+incompatible

replace github.com/firecracker-microvm/firecracker-containerd => github.com/ustiugov/firecracker-containerd v0.0.0-20200709144213-2a6e26bab456

require (
	github.com/containerd/containerd v1.3.5-0.20200521195814-e655edce10c9
	github.com/golang/protobuf v1.3.3
	github.com/sirupsen/logrus v1.5.0
	github.com/stretchr/testify v1.5.1
<<<<<<< 5546571a53610fb3271827582e58124471bf2fc6
<<<<<<< 603848f2670cd27add454d4d1f2a3a8e75e35b28
	github.com/ustiugov/fccd-orchestrator/ctriface v0.0.0-20200617163712-2c7eaa4ce152
	github.com/ustiugov/fccd-orchestrator/misc v0.0.0-20200617163712-2c7eaa4ce152
=======
	github.com/ustiugov/fccd-orchestrator/ctriface v0.0.0-20200706135218-7a63ebbc2626
=======
	github.com/ustiugov/fccd-orchestrator/ctriface v0.0.0-20200709160332-9aecfd5d8df2
>>>>>>> orch tests passing with updated containerd
	github.com/ustiugov/fccd-orchestrator/misc v0.0.0-20200608162316-88962af36173
>>>>>>> added test for multiple loads and invocations
	github.com/ustiugov/fccd-orchestrator/proto v0.0.0-20200421101715-3d8808b0d980
	golang.org/x/sync v0.0.0-20200317015054-43a5402ce75a
	google.golang.org/grpc v1.28.0
)
