## vHive developer's guide

*Note: this page is going to be extended.*

## Deploying single node container environment

You can use the image to build/test/develop vHive inside a container. This image is preconfigured to run a single node Kubernetes cluster inside a container and contains packages to setup vHive on top of it. 
```bash
git clone -b custom_docker_params_for_vHive https://github.com/ease-lab/kind
# build kind
cd kind && go build
# pull latest image
docker pull vhiveease/vhive_dev_env
# Start a container 
kind create cluster --image vhiveease/vhive_dev_env
# Enter the container
docker exec -it <container name> bash
```
### Clean up
```bash
# list all kind clusters
kind get clusters 
# delete a cluster 
kind delete cluster --name <name>
```

Once the container is up and running, follow [this](./quickstart_guide.md#setup-a-single-node-cluster-master-and-worker-functionality-on-the-same-node) guide to setup a single node vHive cluster.


## High-level features

* vHive supports both the baseline Firecracker snapshots and our advanced
Record-and-Prefetch (REAP) snapshots.

* vHive integrates with Kubernetes and Knative via its built-in CRI support.

* vHive supports arbitrary distributed setup of a serverless cluster.

* vHive supports arbitrary functions deployed with OCI (Docker images).

* vHive has robust Continuous-Integration and our team is committed to deliver
high-quality code.

### Deploying a MinIO S3 service in a cluster

```bash
# create a folder in the local storage (on <MINIO_NODE_NAME> that is one of the Kubernetes nodes)
sudo mkdir -p <MINIO_PATH>

cd ./configs/storage/minio

# create a persistent volume (PV) and the corresponding PV claim
# specify the node name that would host the MinIO objects
# (use `hostname` command for the local node)
MINIO_NODE_NAME=<MINIO_NODE_NAME> MINIO_PATH=<MINIO_PATH> envsubst < pv.yaml | kubectl apply -f -
kubectl apply -f pv-claim.yaml
# create a storage app and the corresponding service
kubectl apply -f deployment.yaml
kubectl apply -f service.yaml
```

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

* vHive uses a [fork](https://github.com/ease-lab/kind) of [kind](https://github.com/kubernetes-sigs/kind) to speed up testing environment setup requiring Kubernetes.
