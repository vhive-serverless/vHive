# vHive local registry guide

To avoid bottlenecks, it is possible to use a local registry to store images. This registry is reachable at *docker-registry.registry.svc.cluster.local:5000*.

## Pulling images to the local registry

1. Create a txt file containing the images that need to be pulled to the local registry

2. Pull the images to the local registry. For docker the following command can be used from [vSwarm](https://github.com/vhive-serverless/vSwarm):

   `go run tools/registry/populate_registry.go -imageFile images.txt -source docker://docker.io`

## Using the local registry

Once the desired images are available at the local registry, it can be used in function deployment by specifying the registry in the image name. When no registry is specified, the registry defaults to *docker.io*.

Example: `docker-registry.registry.svc.cluster.local:5000/vhiveease/helloworld:var_workload`

