## vHive developer's guide

*Note: this page is going to be extended.*


## High-level features

* vHive supports both the baseline Firecracker snapshots and our advanced
Record-and-Prefetch (REAP) snapshots.

* vHive integrates with Kubernetes and Knative via its built-in CRI support.

* vHive supports arbitrary distributed setup of a serverless cluster.

* vHive supports arbitrary functions deployed with OCI (Docker images).

* vHive has robust Continuous-Integration and our team is committed to deliver
high-quality code.


## Performance analysis

Currently, vHive supports two modes of operation that enable different types
of performance analysis:

* [Distributed setup.](./quickstart_guide.md)
Allows analysis of the end-to-end performance based on the statistics provided by
the [invoker client](../examples/README.md).

* [Single-node setup.](../bench_test.go)
A test that is integrated with vHive-CRI orchestrator via a programmatic interface
allows to analyze latency breakdown of boot-based and snapshot cold starts,
using detailed latency and memory footprint metrics.


## Dependencies and binaries

* vHive uses Firecracker-Containerd binaries that are build using the `user_page_faults` branch
of our [fork](https://github.com/ease-lab/firecracker-containerd) of the upstream repository.
Currently, we are in the process of upstreaming VM snapshots support to the upstream repository.

* Current Firecracker version is 0.21.0. We plan to keep our code loosely up to date with
the upstream Firecracker repository.
