## vHive developer's guide

*Note: this page is going to be extended.*

## Deploying single node container environment

You can use the image to build/test/develop vHive inside a container. This image is preconfigured to run a single node Kubernetes cluster inside a container and contains packages to setup vHive on top of it. 
```bash
# Set up the host (the same script as for the self-hosted GitHub CI runner)
./scripts/github_runner/setup_runner_host.sh
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

## Testing stock Knative images

If you need to test your Knative images on stock Knative environment, use the commands below to setup an environment for that.
```bash
git clone https://github.com/ease-lab/vhive
cd vhive
./scripts/cloudlab/setup_node.sh stock-only
sudo containerd
./scripts/cluster/create_one_node_cluster.sh stock-only
# wait for the containers to boot up using 
watch kubectl get pods -A
# once all the containers are ready/complete, you may start Knative functions
kn service apply
```
### Clean up
```bash
./scripts/github_runner/clean_cri_runner.sh stock-only
```

## High-level features

* vHive supports both the baseline Firecracker snapshots and our advanced
Record-and-Prefetch (REAP) snapshots.

* vHive integrates with Kubernetes and Knative via its built-in CRI support.
Currently, only Knative Serving is supported.

* vHive supports arbitrary distributed setup of a serverless cluster.

* vHive supports arbitrary functions deployed with OCI (Docker images).

* vHive has robust Continuous-Integration and our team is committed to deliver
high-quality code.


### MinIO S3 service

#### Deploying a MinIO service

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

#### Deleting the MinIO service that was created with the instructions above

```bash
kubectl delete deployment minio-deployment
kubectl delete pvc minio-pv-claim
kubectl delete svc minio-service
kubectl delete pv minio-pv
```

Note that files in the bucket persist in the local filesystem after a persistent volume removal.


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

## Knative request tracing
Knative function call requests can now be traced & visualized using [zipkin](https://zipkin.io/). Zipkin is a distributed tracing system featuring easy collection and lookup of tracing data. Checkout [this](https://www.scalyr.com/blog/zipkin-tutorial-distributed-tracing/) for a quickstart guide.

* Once the zipkin container is running, start the dashboard using `istioctl dashboard zipkin`.
* To access requests remotely, run `ssh -L 9411:127.0.0.1:9411 <Host_IP>` for port forwarding.
* Go to your browser and enter [localhost:9411](http://localhost:9411) for the dashboard.

## Dependencies and binaries

* vHive uses Firecracker-Containerd binaries that are build using the `user_page_faults` branch
of our [fork](https://github.com/ease-lab/firecracker-containerd) of the upstream repository.
Currently, we are in the process of upstreaming VM snapshots support to the upstream repository.

* Current Firecracker version is 0.21.0. We plan to keep our code loosely up to date with
the upstream Firecracker repository.

* vHive uses a [fork](https://github.com/ease-lab/kind) of [kind](https://github.com/kubernetes-sigs/kind) to speed up testing environment setup requiring Kubernetes.
