# Changelog

## [Unreleased]

### Added

### Changed

### Fixed

## Release v1.8

### Added

### Changed

- Upgraded supported Ubuntu version to 24.04.
- Bump Go to 1.22.

### Fixed

- Fix IP choice for CloudLab clusters to use the internal network interface for control plane communication.
- Fix disk issues on CloudLab profiles after restart.
- Fix iptables issues on Ubuntu 22.04, 24.04.
- Improved networking for Firecracker uVMs.

## Release v1.7.1

### Added
- Added support for arm64 ubuntu 18.04 in stock-only setup with [setup scripts](./scripts/setup.go).
- Added support for [`K8 Power Manager`](https://networkbuilders.intel.com/solutionslibrary/power-manager-a-kubernetes-power-operator-technology-guide), a Kubernetes operator designed to manage and optimize power consumption in a Kubernetes cluster. More details are described [here](docs/power_manager.md).

## Changed

## Fixed
- Fixed critical issue with IP choice that made CloudLab clusters use the external network interface for control plane communication.

### Release v1.7

### Added

- Added support for [`OpenYurt`](https://openyurt.io/), an open platform that extends upstream Kubernetes to run on edge node pools. More details on how to the `Knative-atop-OpenYurt` mode are described [here](scripts/openyurt-deployer/README.md).


### Changed

- Removed the utils and examples from the vHive repo, moved to [vSwarm](https://github.com/vhive-serverless/vSwarm).
- Bumped Go to 1.21, Kubernetes to v1.29, Knative to v1.13, Istio to 1.20.2, MetalLB to 0.14.3, Calico to 3.27.3.
- Made the automatic patching of the knative-serving and calico manifests instead of storing the patched manifests in the repo.

### Fixed

## v1.6

### Added
- Added support for [NVIDIA GPU](https://docs.nvidia.com/datacenter/cloud-native/kubernetes/install-k8s.html) in stock-only setup, with [setup script](./scripts/gpu/setup_nvidia_gpu.sh) and [example](./configs/gpu/gpu-function.yaml) Knative deployment 
- Upgraded the Firecracker version.  [Vanilla Firecracker snapshots](./docs/snapshots.md) are
  supported with local snapshot storage. Remote snapshot support is added but unstable (GH-823).
- Added a new `netPoolSize` option to configure the amount of network devices in the Firecracker VM network pool (`10`
  by default), which can be used to keep the network initialization off the cold start path of Firecracker VMs.
### Changed

- Changed [system setup script](./scripts/setup_system.sh). NVIDIA helm is now one of the vHive dependencies.
- Disabled the UPF feature for Firecracker snapshots (GH-807), but it is still available in the
  [legacy branch](https://github.com/vhive-serverless/vHive/tree/legacy-firecracker-v0.24.0-with-upf-support).
- Update [quick start guide](./docs/quickstart_guide.md) to use refactored Go version [setup scripts](./scripts/setup.go) with a unified entry for easily setting up vHive and remove some legacy bash scripts under [scripts](./scripts/)

### Fixed

- Removed the limitation on the number of functions instances that can be restored from a single Firecracker snapshot
  (previously it was limited to `1`).


## v1.5

### Added
- Added support for [eStargz](https://github.com/containerd/stargz-snapshotter) in stock-only setup.
- Added [setup script](./scripts/stargz/setup_stargz.sh) for stargz-snapshotter.
- Added [example](./configs/knative_workloads/stargz-node.yaml) knative deployment for eStargz.

### Changed
- Bumped Knative to v1.9, Go to v1.19, Kubernetes to v1.25, Istio to 1.16.0, metallb 0.13.9, Calico to 3.25.1
- Bumped the GitHub-hosted runner OS version to ubuntu 20.

### Fixed


## v1.4.2

### Added

### Changed
- Bumped Knative to v1.4, Go to v1.18, Kubernetes to v1.23, Istio to 1.12, protoc to 3.19, runc to 1.1.

### Fixed


## v1.4.1

### Added

### Changed
- Support Ubuntu 20 as the host OS, Ubuntu 18 support dropped.
- Removed local registry CRI tests.

### Fixed


## v1.4

### Added

- Added support for [gVisor](https://gvisor.dev) MicroVMs, as an alternative to Firecracker.
- Added [vSwarm](https://github.com/vhive-serverless/vSwarm), a suite of representative serverless workloads.
Currently, in a beta testing mode.
- Added Python and Go tracing modules and an example showing its usage.
Moved to [vSwarm](https://github.com/vhive-serverless/vSwarm/tree/main/utils/tracing).
- Added Golang and Python storage modules, supporting AWS S3 and AWS ElastiCache.
Moved to [vSwarm](https://github.com/vhive-serverless/vSwarm/tree/main/utils/storage).
- Added self-hosted stock-Knative runners on KinD,
see [`scripts/self-hosted-kind`](./scripts/self-hosted-kind/).

### Changed

- Workload stdout/stderr is not directly redirected to vhive stdout/stderr anymore
but is printed by vhive via `logrus.WithFields(logrus.Fields{"vmID": vmID})`.
- Moved the CRI non-Firecracker tests to self-hosted stock-Knative runners.


## v1.3

### Added

- Added 2 chained-functions microbenchmarks, synchronous and asynchronous, that use Knative Serving and Eventing, correspondingly.
Tracing is fully supported for Serving function composition, partially supported for Eventing function composition.
- Added documentation on vHive benchmarking methodology
for arbitrary serverless deployments.
- Added documentation for adding benchmarks to vHive.
- Added Knative Eventing Tutorial: [documentation](./docs/knative/eventing.md) and [example](https://github.com/vhive-serverless/vSwarm/blob/main/tools/knative-eventing-tutorial).
- Added a Go module for tracing using zipkin.
- Improved CI troubleshooting: CRI test logs are now stored as GitHub artifacts.
- Added [script](https://github.com/vhive-serverless/vHive/blob/main/docs/quickstart_guide.md#iii-setup-a-single-node-cluster) to (re)start vHive single node cluster in a push-button.
- Added a linter for hyperlink checking in markdown files.

### Changed

- Bumped Containerd to v1.6.0.
- Bumped Knative to v0.23.0.
- Bumped Go to v1.16.4.
- Frozen Kubernetes at v1.20.6-00.
- Simplified Go dependencies management by refactoring modules into packages.

### Fixed

- Fixed stock Knative cluster startup.


## v1.2

### Added

Features for **performance analysis**
- Zipkin support added for tracing and breaking down latencies in a distributed vHive setting (e.g., across Istio and Knative services).
More info [here](./docs/developers_guide.md#Knative-request-tracing)
- [beta] Added a profiler that collects low-level microarchitectural metrics,
using the Intel [TopDown](https://ieeexplore.ieee.org/document/6844459) method.
The tool aims at studying the implications of multi-tenancy, i.e., the VM number,  on the the tail latency and throughput.

Features for **benchmarking at scale** and **multi-function applications**
- Added cluster-local container registry support to avoid DockerHub bottleneck. Contributed by @amohoste from ETH Zurich.
- [alpha] Added Knative eventing support using In-Memory Channel and MT-Channel-broker.
Integration tests and Apache Kafka support coming soon.
- Added support for MinIO object store (non-replicated, non-distributed).
More info [here](./docs/developers_guide.md#MinIO-S3-service)

Other
- vHive now also supports vanilla Knative benchmarking and testing (i.e., using containers for function sandboxes).
More info [here](./docs/developers_guide.md#Testing-stock-Knative-images).

### Changed
- Bumped up the Firecracker version to v0.24 with REAP snapshots support.
- Bumped up all Knative components to version v0.21.
- MicroVMs have network access to all services deployed in a vHive/k8s cluster and the Internet by default,
using an automatically detected, or a user-specified, host interface.

### Fixed
- CI pulls the latest binaries from git-lfs when running tests on self-hosted runners.

## v1.1

### Added

- Created a CI pipeline that runs CRI integration tests inside containers using [kind](https://kind.sigs.k8s.io/).
- Added a developer image that runs a vHive k8s cluster in Docker, simplifying vHive development and debugging.
- Extended the developers guide on the modes of operation, performance analysis and vhive development environment inside containers.
- Added a slide deck of Dmitrii's talk at Amazon.
- Added a commit linter and a spell checker for `*.md` files.

### Changed

- Use replace pragmas instead of Go modules.
- Bump Go version to 1.15 in CI.
- Deprecated Travis CI.

### Fixed

- Fixed the vHive cluster setup issue for clusters with >2 nodes [issue](https://github.com/vhive-serverless/vhive/issues/94).
