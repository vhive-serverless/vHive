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


## Dependencies and binaries

* vHive uses Firecracker-Containerd binaries that are build using the user_page_faults`branch
of our [fork](https://github.com/ease-lab/firecracker-containerd) of the upstream repository.
Currently, we are in the process of upstreaming VM snapshots support to the upstream repository.

* Current Firecracker version is 0.21.0. We plan to keep our code loosely up to date with
the upstream Firecracker repository.
