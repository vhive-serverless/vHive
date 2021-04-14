[![build](https://github.com/ease-lab/vhive/workflows/vHive%20build%20tests/badge.svg)](https://github.com/ease-lab/vhive/actions)
[![Unit Tests](https://github.com/ease-lab/vhive/workflows/vHive%20unit%20tests/badge.svg)](https://github.com/ease-lab/vhive/actions)
[![Integration Tests](https://github.com/ease-lab/vhive/workflows/vHive%20integration%20tests/badge.svg)](https://github.com/ease-lab/vhive/actions)
[![Nightly Tests](https://github.com/ease-lab/vhive/workflows/vHive%20nightly%20integration%20tests/badge.svg)](https://github.com/ease-lab/vhive/actions)
[![Go Report Card](https://goreportcard.com/badge/github.com/ease-lab/vhive)](https://goreportcard.com/report/github.com/ease-lab/vhive)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

![vHive Header](docs/figures/vhive_hdr.jpg)

## Mission

vHive aims to enable serverless systems researchers to innovate across the deep and distributed software stacks
of a modern serverless platform. Hence, we built vHive to be representative of the leading
Function-as-a-Service (FaaS) providers, integrating the same production-grade components used by the providers, including
[AWS Firecracker hypervisor](https://firecracker-microvm.github.io/),
Cloud Native Computing Foundation's [Containerd](https://containerd.io/),
and [Kubernetes](https://kubernetes.io/).

vHive adopts the [Knative](https://knative.dev/) flexible programming model, allowing the researchers to quickly deploy
and experiment with *any* serverless applications that may comprise many functions,
running in secure Firecracker microVMs, as well as serverfull services.
Both the functions and the stateful services can be deployed using OCI/Docker images.

vHive empowers systems researchers to innovate on key serverless features,
including functions autoscaling and cold-start delay optimization with several snapshotting mechanisms.


## vHive architecture

![vHive Architecture](docs/figures/vhive_architecture.jpg)

The details of the vHive architecture can be found in our ASPLOS'21 paper
([extended abstract](https://asplos-conference.org/abstracts/asplos21-paper212-extended_abstract.pdf),
[full paper](docs/papers/REAP_ASPLOS21.pdf)).


### Technical talks

* [Slides](docs/talks/vHive_REAP_@AWS_04_02_2021.pdf) from
[Dmitrii](http://homepages.inf.ed.ac.uk/s1373190/)'s talk at AWS on Feb, 4th 2021.


## Referencing our work

If you decide to use vHive for your research and experiments, we are thrilled to support you by offering
advice for potential extensions of vHive and always open for collaboration.

Please cite our paper that has been recently accepted to ASPLOS 2021:
```
@inproceedings{ustiugov:benchmarking,
  author    = {Dmitrii Ustiugov and
               Plamen Petrov and
               Marios Kogias and
               Edouard Bugnion and
               Boris Grot},
  title     = {Benchmarking, Analysis, and Optimization of Serverless Function Snapshots},
  booktitle = {Proceedings of the 26th ACM International Conference on
               Architectural Support for Programming Languages and Operating Systems (ASPLOS'21)},
  publisher = {{ACM}},
  year      = {2021},
  doi       = {10.1145/3445814.3446714},
}
```


## Getting started with vHive

vHive can be readily deployed on premises or in cloud, with support for nested virtualization.
We provide [a quick-start guide](docs/quickstart_guide.md)
that describes how to set up an experiment with vHive.


## Developing vHive

### Developer's guide and performance analysis with vHive

We provide a basic developer's guide that we plan to extend in future.
We encourage the community to contribute their analysis scenarios and tools.

### vHive roadmap

vHive is a community-led project, maintained by the EASE lab.
The current roadmap is [available](https://github.com/ease-lab/vhive/projects/1)
and is going to be updated to accommodate the community's goals and evolution.
To guarantee high code quality and reliability, we deploy fully automated CI
on cloud and self-hosted runners with GitHub Actions.

The statistics of this repository's views, clones, forks is available by the following
[link](https://ease-lab.github.io/vhive.github.io/ease-lab/vhive/latest-report/report.html).


### Getting help and contributing

We would be happy to answer any questions in GitHub Issues and encourage the open-source community
to submit new Issues, assist in addressing existing issues and limitations, and contribute their code with Pull Requests.
You can also talk to us on the official [Firecracker Slack](https://join.slack.com/t/firecracker-microvm/shared_invite/zt-oxbm7tqt-GLlze9zZ7sdRSDY6OnXXHg) in the **#firecracker-vHive-research** channel.


## License and copyright

vHive is free. We publish the code under the terms of the MIT License that allows distribution, modification, and commercial use.
This software, however, comes without any warranty or liability.

The software is maintained at the [EASE lab](https://easelab.inf.ed.ac.uk/) in the University of Edinburgh.


### Maintainers

* Dmitrii Ustiugov: [GitHub](https://github.com/ustiugov),
[twitter](https://twitter.com/DmitriiUstiugov), [web page](http://homepages.inf.ed.ac.uk/s1373190/)
* [Plamen Petrov](https://github.com/plamenmpetrov)

