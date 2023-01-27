# Changelog

## [Unreleased]

### Added

### Changed

### Fixed


## v1.5

### Added

### Changed
- Bumped Containerd to v1.6, Knative to v1.9, Go to v1.18, Kubernetes to v1.23, Istio to 1.12, protoc to 3.19, runc to 1.1.

### Fixed


## v1.4.1

### Added

### Changed
- Support Ubuntu 20 as the host OS, Ubuntu 18 support dropped.

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
- Added Knative Eventing Tutorial: [documentation](./docs/knative/eventing.md) and [example](./examples/knative-eventing-tutorial).
- Added a Go module for tracing using zipkin.
- Improved CI troubleshooting: CRI test logs are now stored as GitHub artifacts.
- Added [script](./scripts/cloudlab/start_onenode_vhive_cluster.sh) to (re)start vHive single node cluster in a push-button.
- Added a linter for hyperlink checking in markdown files.

### Changed

- Bumped Containerd to v1.5.2.
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
