# Running stargz-based containers with Knative

This guide describes how to run stargz images on a single-cluster node using Knative's CLI.

## Creating stargz images
[eStargz](https://github.com/containerd/stargz-snapshotter/tree/cmd/v0.12.1) is a lazily-pullable image format developed to improve the performance of container boot-ups by making better usage of the layering structure of container images. The image format is compatible to [OCI](https://github.com/opencontainers/image-spec/)/[Docker](https://github.com/moby/moby/blob/master/image/spec/v1.2.md) images, therefore it allows pushing images to standard container registries.

To build stargz images, we recommend following the [stargz snapshotter and stargz store guide](https://github.com/containerd/stargz-snapshotter/blob/cmd/v0.12.1/docs/INSTALL.md) and building images using the [ctr-remote](https://github.com/containerd/stargz-snapshotter/tree/cmd/v0.12.1#creating-estargz-images-using-ctr-remote) CLI tool. We recommend serving images through DockerHub.

## Cluster setup for stargz
Execute the following below as a **non-root** user with **sudo rights** using bash:
1. Setup the kubelet without firecracker and vHive with the `stock-only` and `use-stargz` flags:
    ```bash
    ./scripts/cloudlab/setup_node.sh stock-only use-stargz
    ```
2. Setup single node cluser:
    ```bash
    ./scripts/cluster/create_one_node_cluster.sh stock-only
    ```
3. Deploy the Knative service:
    ```bash
    kn service apply <name> -f <yaml_config_path> --concurrency-target 1
    ```
    Note: We provide an [example yaml file](../../configs/knative_workloads/stargz-node.yaml) that creates a NodeJS pod.
4. Delete deployed service(s):
    ```bash
    kn service delete --all
    ```