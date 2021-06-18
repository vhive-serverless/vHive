## Dockerized functions that are ready to use with vHive

All functions are written in Python and packaged as OCI/Docker images on top of Alpine Linux
(except for Video Processing that is based on debian-slim due to the problem of installing OpenCV on Alpine).

To build an image, type (you can build one or several images by providing a space-separated list
of the names of their corresponding folders):

```
export DOCKERHUB_ACCOUNT="your DockerHub account ID"
bash docker_build.sh image1 image2 image3
```

Some workloads (with `_s3` postfix in the folder name) require a [MinIO](https://min.io/), which is an open source S3-compatible object store, server
deployed on the same host. We provide a set of scripts for MinIO setup in the `minio_scripts` folder.


### Credits

Functions' code is adopted from [FunctionBench](https://github.com/kmu-bigdata/serverless-faas-workbench),
a representative suite of serverless functions.
