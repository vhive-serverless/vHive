## vHive developer's guide

*Note: this page is going to be extended.*

## Testing stock Knative setup or images

If you need to test Knative functions in stock Knative environment (i.e., containers)
or in gVisor MicroVMs instead of Firecracker MicroVMs, use the following commands to set up the environment.

```bash
git clone https://github.com/vhive-serverless/vhive
cd vhive
./scripts/cloudlab/setup_node.sh [stock-only|gvisor|firecracker]
sudo containerd
./scripts/cluster/create_one_node_cluster.sh [stock-only|gvisor|firecracker]
# wait for the containers to boot up using
watch kubectl get pods -A
# once all the containers are ready/complete, you may start Knative functions
kn service apply
```

To deploy a function in the stock or gVisor execution environment, please create and use your own YAML files following this [example](https://github.com/knative/docs/tree/main/code-samples/serving/hello-world/helloworld-python#yaml)

### Clean up
```bash
./scripts/github_runner/clean_cri_runner.sh stock-only
```

## Deploying single node container environment

You can use the image to build/test/develop vHive inside a [kind container](https://github.com/ease-lab/kind).
This image is preconfigured to run a single node Kubernetes cluster
inside a container and contains packages to setup vHive on top of it.

```bash
# Set up the host (the same script as for the self-hosted GitHub CI runner)
./scripts/github_runner/setup_integ_runners_host.sh
# pull latest image
docker pull vhiveease/vhive_dev_env
# Start a container
kind create cluster --image vhiveease/vhive_dev_env
```

### Running a vHive cluster in a kind container.
Before running a cluster, one might need to install additional tools, e.g., Golang,
and check out the vHive repository manually.

```bash
# Enter the container
docker exec -it <container name> bash
# Inside the container, create a single-node cluster
./scripts/cluster/create_one_node_cluster.sh [stock-only]
```
> **Notes:**
>
> When running a vHive, or stock Knative, cluster inside a kind container,
> one should not run setup scripts but start the daemon(s) and create the cluster right away.
>
> Currently, with Firecracker, running only a single-node cluster is supported ([Issue](https://github.com/vhive-serverless/vhive/issues/126) raised).
> Running a multi-node cluster with stock Knative should work but is not tested.

### Clean up

```bash
# list all kind clusters
kind get clusters
# delete a cluster
kind delete cluster --name <name>
```

## Testing using stock-Knative on self-hosted KinD runners (used for the vHive CI)
We also offer self-hosted stock-Knative environments powered by KinD. To be able to use them, follow the instructions below:

- [ ] Set [`jobs.<job_id>.runs-on`](https://docs.github.com/en/actions/reference/workflow-syntax-for-github-actions#jobsjob_idruns-on) to `stock-knative`.
- [ ] For your GitHub workflow, define `TMPDIR` environment variable in your manifest:
    ```yaml
    env:
      TMPDIR: /root/tmp
    ```
    - [ ] As the first step of **all** jobs, "create `TMPDIR` if not exists":
        ```yaml
        jobs:
          my-job:
            name: My Job
            runs-on: [stock-knative]

            steps:
              - name: Setup TMPDIR
                run: mkdir -p $TMPDIR
        ```
- [ ] Make sure to **clean-up and wait** for it to end! This varies for each workload, but below are some examples:
    ```yaml
    jobs:
      my-job:
      name: My Job
      runs-on: [stock-knative]

      steps:
        # ...

        name: Cleaning
        if: ${{ always() }}
        run: |
          # ...
    ```
    - If you have used `kubectl apply -f ...` then use `kubectl delete -f ...`
    - If you have used `kn service apply` then use `kn service delete -f ... --wait`

## Running CRI tests locally
Assuming you rented a node using the vHive CloudLab profile:

1. Setup the node for the desired sandbox:

```bash
./scripts/cloudlab/setup_node.sh <firecracker|gvisor>
```

2. Setup the CRI test environment for the desired sandbox:

```bash
./scripts/github_runner/setup_cri_test_env.sh <firecracker|gvisor>
```

3. Run CRI tests:

```bash
source /etc/profile && go clean -testcache && go test ./cri -v -race -cover
```

4. Cleanup:

```bash
./scripts/github_runner/clean_cri_runner.sh <firecracker|gvisor>
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
Knative function call requests can now be traced & visualized using [zipkin](https://zipkin.io/).
Zipkin is a distributed tracing system featuring easy collection and lookup of tracing data.
Here are some useful commands (there are plenty of Zipkin tutorials online):

* Once the zipkin container is running, start the dashboard using `istioctl dashboard zipkin`.
* To access requests remotely, run `ssh -L 9411:127.0.0.1:9411 <Host_IP>` for port forwarding.
* Go to your browser and enter [localhost:9411](http://localhost:9411) for the dashboard.


## Dependencies and binaries

* vHive uses Firecracker-Containerd binaries that are build using the `user_page_faults` branch
of our [fork](https://github.com/vhive-serverless/firecracker-containerd) of the upstream repository.
Currently, we are in the process of upstreaming VM snapshots support to the upstream repository.

* Current Firecracker version is 0.24.0, Knative 1.9, Kubernetes 1.25.3, gVisor 20210622.0, and Istio 1.16.0.
We plan to keep our code loosely up to date with the upstream Firecracker repository.

* vHive uses a [fork](https://github.com/ease-lab/kind) of [kind](https://github.com/kubernetes-sigs/kind)
to speed up testing environment setup requiring Kubernetes.

* Current [eStargz](https://github.com/containerd/stargz-snapshotter) version is 0.13.0.


## Deploying Knative functions that require GPU Support

Knative functions can use GPU although only `stock-only` mode is supported. 

### Install Stock Kubernetes and Knative

Follow the guide to [setup stock knative](#testing-stock-knative-setup-or-images). 

``` bash
./scripts/cloudlab/setup_node.sh stock-only
```

### Install NVIDIA Driver and NVIDIA Container Toolkit 
The script will install NVIDIA CUDA Driver and assume there’s no NVIDIA driver currently running. 

You can use the script provided if the install of containerd is using our script or manually edit the containerd settings following [NVIDIA's official document](https://docs.nvidia.com/datacenter/cloud-native/kubernetes/install-k8s.html#install-nvidia-container-toolkit-nvidia-docker2).

The script has been tested on ubuntu20.04, with GPU including NVIDIA A100, V100 or P100.

``` bash
./scripts/gpu/setup_nvidia_gpu.sh
```


### Start Containerd and Knative

``` bash
sudo screen -dmS containerd containerd; sleep 5;
./scripts/cluster/create_one_node_cluster.sh stock-only
```

### Install NVIDIA Device Plugin

Using helm to install the NVIDIA Device plugin after all pods are running or completed.

``` bash
helm install --generate-name -n nvidia-device-plugin --create-namespace nvdp/nvidia-device-plugin
```
At this point, all pods should be successfully deployed.
``` bash
$ kubectl get pods -A
NAMESPACE              NAME                                       READY   STATUS      RESTARTS   AGE
nvidia-device-plugin   nvidia-device-plugin-1684892866-rtgk9      1/1     Running     0          80s
```
And you can use the gpu-pod.yaml to test whether the NVIDIA Device Plugin works

``` bash
kubectl apply -f ./configs/gpu/gpu-pod.yaml
```
After that, check the log to contain:

``` bash
$ kubectl logs gpu-operator-test
```
``` bash
[Vector addition of 50000 elements]
Copy input data from the host memory to the CUDA device
CUDA kernel launch with 196 blocks of 256 threads
Copy output data from the CUDA device to the host memory
Test PASSED
Done
```
### Test Knative with GPU
Using GPU in Knative is simple and similar as for a regular Kubernetes service. The only change is to add the limits of GPU to the YAML file.

``` yaml
resources:
  limits:
    nvidia.com/gpu: 1
```
You can also deploy our example container, which is a Golang function that calls and returns the output of `nvidia-smi`.

``` bash
kn service apply -f ./configs/gpu/gpu-function.yaml
```
Once the service has been deployed successfully, you can call it and check it's response.
``` bash
curl "$(kn service describe hello-gpu -o URL)"
```
``` bash
Hello GPU!
Wed May 24 02:04:40 2023       
+---------------------------------------------------------------------------------------+
| NVIDIA-SMI 530.30.02              Driver Version: 530.30.02    CUDA Version: 12.1     |
+---------------------------------------------------------------------------------------+

```
