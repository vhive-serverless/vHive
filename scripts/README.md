# vHive Setup Scripts

- [vHive Setup Scripts](#vhive-setup-scripts)
    - [1. Get Setup Tool](#1-get-setup-tool)
        - [1.1 Download the Binary Executable Directly](#11-download-the-binary-executable-directly)
        - [1.2 Build from Source](#12-build-from-source)
    - [2. Config the Setup Tool](#2-config-the-setup-tool)
    - [3. Use of Setup Tool](#3-use-of-setup-tool)
        - [3.1 General Usage](#31-general-usage)
        - [3.2 Specify Config Files](#32-specify-config-files)
        - [3.3 Use with Local vHive Repo](#33-use-with-local-vhive-repo)
        - [3.4 Use with Remote vHive Repo (Standalone Use)](#34-use-with-remote-vhive-repo-standalone-use)
        - [3.5 Migrate from Legacy Shell Scripts](#35-migrate-from-legacy-shell-scripts)
    - [4. Logs](#4-logs)
    - [5. Supported Platform](#5-supported-platform)

## 1. Get Setup Tool

There are basically two ways to get the setup tool

### 1.1 Download the Binary Executable Directly

Check [vHive GitHub Repo](https://github.com/vhive-serverless/vHive/releases) for more details and choose the
appropriate version to download.

### 1.2 Build from Source

**Building from source requires Go (version 1.19 at least) installed on your system.**

```bash
git clone --depth 1 https://github.com/vhive-serverless/vHive
cd vHive
pushd scripts && go build -o setup_tool && popd
```

Compiled executable file will be in the `scripts` directory.

## 2. Config the Setup Tool

**Normally, just skip this section and use the default config files** which are located in the `configs/setup` directory
inside the [vHive repo](https://github.com/vhive-serverless/vHive).

- `configs/setup/knative.json`: knative related configs (all the path in the config file should be relative path inside
  the vHive repo)
- `configs/setup/kube.json`: Kubernetes related configs
- `configs/setup/system.json`: system related configs
- `configs/setup/vhive.json`: vHive related configs

You can modify the config files on your demand and then place all of them in one directory for the later use.

## 3. Use of Setup Tool

### 3.1 General Usage

```bash
./setup_tool [options] <subcommand> [parameters]
```

use the `-h` or `--help` option to look for the help

### 3.2 Specify Config Files

By default, the setup_tool will use the config files in `configs/setup` directory inside the vHive repo.

To change the path of config files, use the `--setup-configs-dir` option to specify it.

```bash
./setup_tool --setup-configs-dir <CONFIG PATH> ...
```

### 3.3 Use with Local vHive Repo

By default, the setup_tool will check the current directory to ensure it is a vHive repo and then use it during the
setup process.

To use other vHive repos locally, provide the `--vhive-repo-dir` option to specify it.

```bash
./setup_tool --vhive-repo-dir <VHIVE REPO PATH> ...
```

If the current directory or the provided path is not a valid vHive repo, the setup_tool
will [automatically clone the remote vHive repo and use it](#34-use-with-remote-vhive-repo).

### 3.4 Use with Remote vHive Repo (Standalone Use)

When the setup_tool is directly downloaded or targeted for standalone use, the setup_tool will automatically clone the
remote vHive repo to the temporary directory and then use it during the setup process.

To change the URL and branch of the [default remote vHive repo](https://github.com/vhive-serverless/vHive),
use `--vhive-repo-url` and `--vhive-repo-branch` options to specify them.

```bash
./setup_tool --vhive-repo-url <URL> --vhive-repo-branch <BRANCH> ...
``` 

Besides, when the current directory is a vHive repo or the `--vhive-repo-dir` option is valid, **the local repo will be
prioritized for use**. **To force the setup_tool to clone and use the remote vHive repo**, provide `--force-remote`
option to the setup_tool.

```bash
./setup_tool --force-remote ...
```

### 3.5 Migrate from Legacy Shell Scripts

Just type the name of the original shell script and append corresponding parameters behind. For example:

```bash
# Legacy ==>
scripts/cloudlab/setup_node.sh stock-only
# ==> Current
./setup_tool [options] setup_node REGULAR stock-only

# Legacy ==>
scripts/create_devmapper.sh
# ==> Current
./setup_tool [options] create_devmapper

# Legacy ==>
scripts/gpu/setup_nvidia_gpu.sh
# ==> Current
./setup_tool [options] setup_nvidia_gpu
```

**NOTICE**: Shell scripts in `scripts/stargz`, `scripts/self-hosted-kind`, and `scripts/github_runner` **are not
supported to be invoked in this way at present**.

### 3.6 Kubernetes Control Plane High-Availability Mode

For fault tolerance purposes, Kubernetes control plane components can be replicated. This can be done by combining
instructions from
the following links - [#1](https://github.com/kubernetes/kubeadm/blob/main/docs/ha-considerations.md)
and [#2](https://kubernetes.io/docs/setup/production-environment/tools/kubeadm/high-availability/).

While executing these steps manually is complicated, [InVitro Loader](https://github.com/vhive-serverless/invitro)
provides an automatized way of creating a high-available control plane just by configuring `CONTROL_PLANE_REPLICAS`
parameter in `scripts/setup/setup.cfg` and then running `scripts/setup/create_multinode.sh`.

## 4. Logs

The log files will be named as `<subcommand>_common.log` and `<subcommand>_error.log`. All log files will be stored in
the directory where the setup_tool is executed.

- `<subcommand>_common.log`: all output originally writes to `stdout` will be redirected to this log file.
- `<subcommand>_error.log`: all output originally writes to `stderr` will be redirected to this log file.

## 5. Supported Platform

At present, only `Ubuntu 20.04 (amd64)` is officially tested. Other versions of `Ubuntu` may also work, but not
guaranteed. 