# vHive Quickstart
This guide describes how to set up an _N_-node vHive serverless cluster with Firecracker MicroVMs.
See [here][github-toc] to learn where to find table of contents.

To see how to setup a single node cluster with stock-only or gVisor, see [Developer's guide](developers_guide.md).

## Table of Contents
1. [Host platform requirements](#I-host-platform-requirements)
    1. [Hardware](#1-hardware)
    2. [Software](#2-software)
    3. [CloudLab Deployment Notes](#3-cloudlab-deployment-notes)
        1. [CloudLab Profile](#a-cloudlab-profile)
        2. [Nodes to Rent](#b-nodes-to-rent)
2. [Setup a Serverless (Knative) Cluster](#ii-setup-a-serverless-knative-cluster)
    1. [Setup All Nodes](#1-setup-all-nodes)
    2. [Setup Worker Nodes](#2-setup-worker-nodes)
    3. [Configure Master Node](#3-configure-master-node)
    4. [Configure Worker Nodes](#4-configure-worker-nodes)
    5. [Finalise Master Node](#5-finalise-master-node)
3. [Setup a Single-Node Cluster](#iii-setup-a-single-node-cluster)
    1. [Manual](#1-manual)
    2. [Clean Up](#2-clean-up)
    3. [Using a Script](#3-using-a-script)
4. [Deploying and Invoking Functions in vHive](#iv-deploying-and-invoking-functions-in-vhive)
    1. [Deploy Functions](#1-deploy-functions)
    2. [Invoke Functions](#2-invoke-functions)
    3. [Delete Deployed Functions](#3-delete-deployed-functions)
5. [Deploying eStargz-based Functions](#v-deploying-estargz-based-functions)
    1. [Deploy and Invoke Functions](#1-deploy-and-invoke-functions)
    2. [Delete Deployed Function](#2-delete-deployed-function)

## I. Host platform requirements
### 1. Hardware
1. Two x64 servers in the same network.
    - We have not tried vHive with Arm but it may not be hard to port because Firecracker supports Arm64 ISA.
2. Hardware support for virtualization and KVM.
    - Nested virtualization is supported provided that KVM is available.
3. The root partition of the host filesystem should be mounted on an **SSD**. That is critical for snapshot-based cold-starts.
    - We expect vHive to work on machines that use HDDs but there could be timeout-related issues with large Docker images (>1GB).

### 2. Software
1. Ubuntu/Debian with sudo access and `apt` package manager on the host (tested on Ubuntu 20.04).
    - Other OS-es require changes in our setup scripts, but should work in principle.
2. Passwordless SSH. Copy the SSH keys that you use to authenticate on GitHub to all the nodes and
type `eval "$(ssh-agent -s)" && ssh-add` to allow ssh authentication in the background.

### 3. CloudLab Deployment Notes
We suggest renting nodes on [CloudLab][cloudlab] as their service is available to researchers world-wide.

Please make sure that you are using a "bash" shell whenever you connect via ssh to your cluster nodes, otherwise running some of the following commands will prompt a **"Missing name for redirect"** error. If you chose to use CloudLab, this can be done by selecting the current user's profile (upper left corner on any CloudLab page once logged in) --> **Manage account** --> **Default Shell** --> select **"bash"** from the drop down menu --> **Save**. Sometimes the default shell preference gets overwritten therefore, once you connect to a cluster node, check what type of shell you have opened by running the following command:
```
echo $SHELL
```
The expected output should be:
```
/bin/bash
```
If the opened shell is not a "bash" one, you can just type "bash" in the terminal and it will change the current shell to "bash".

#### A. CloudLab Profile
You can use our CloudLab profile [faas-sched/vhive-ubuntu20][cloudlab-pf].

It is recommended to use a base Ubuntu 20.04 image for each node and connect the nodes in a LAN.

#### B. Nodes to Rent
We tested the following instructions by setting up a **2-node** cluster on Cloudlab, using all of the following SSD-equipped machines: `xl170` on Utah, `rs440` on Mass, `m400` on OneLab. `xl170` are normally less occupied than the other two, and users can consider other SSD-based machines too.

SSD-equipped nodes are highly recommended. Full list of CloudLab nodes can be found [here][cloudlab-hw].

## II. Setup a Serverless (Knative) Cluster
### 1. Setup All Nodes
**On each node (both master and workers)**, execute the following instructions below **as a non-root user with sudo rights** using **bash**:
1. Clone the vHive repository
    ```bash
    git clone --depth=1 https://github.com/vhive-serverless/vhive.git
    ```
2. Change your working directory to the root of the repository:
    ```bash
    cd vhive
    ```
3. Create a directory for vHive logs:
    ```bash
    mkdir -p /tmp/vhive-logs
    ```
3. Run the node setup script:
    ```bash
    ./scripts/cloudlab/setup_node.sh > >(tee -a /tmp/vhive-logs/setup_node.stdout) 2> >(tee -a /tmp/vhive-logs/setup_node.stderr >&2)
    ```
    > **BEWARE:**
    >
    > This script can print `Command failed` when creating the devmapper at the end. This can be safely ignored.

    > **Note:**
    >
    > [eStargz](https://github.com/containerd/stargz-snapshotter/tree/cmd/v0.12.1) is a
    > lazily-pullable image format developed to improve the performance of container boot-ups by
    > making better usage of the layering structure of container images. The image format is 
    > compatible to [OCI](https://github.com/opencontainers/image-spec/)/[Docker](https://github.com/moby/moby/blob/master/image/spec/v1.2.md) images, therefore it allows pushing images to 
    > standard container registries.
    > To enable runs with `stargz` images, setup kubelet by adding the `stock-only` and `use-stargz`
    > flags as follows:
    >   ```bash
    >   ./scripts/cloudlab/setup_node.sh stock-only use-stargz > >(tee -a /tmp/vhive-logs/setup_node.stdout) 2> >(tee -a /tmp/vhive-logs/setup_node.stderr >&2)
    >   ```
    > **IMPORTANT**
    > Currently `stargz` is only supported in native kubelet contexts without firecracker. 
    > Therefore, the following steps from this guide must **not** be executed:
    > * `2.3`,
    > * `2.4`,
    > * `2.5`.


### 2. Setup Worker Nodes
**On each worker node**, execute the following instructions below **as a non-root user with sudo rights** using **bash**:
1. Run the script that setups kubelet:
    ```bash
    ./scripts/cluster/setup_worker_kubelet.sh > >(tee -a /tmp/vhive-logs/setup_worker_kubelet.stdout) 2> >(tee -a /tmp/vhive-logs/setup_worker_kubelet.stderr >&2)
    ```
    > **IMPORTANT:**
    > If step `1.3` was executed with the `stock-only` flag, execute the following instead:
    >   ```bash
    >   ./scripts/cluster/setup_worker_kubelet.sh stock-only > >(tee -a /tmp/vhive-logs/setup_worker_kubelet.stdout) 2> >(tee -a /tmp/vhive-logs/setup_worker_kubelet.stderr >&2)
    >   ```
2. Start `containerd` in a background terminal named `containerd`:
    ```bash
    sudo screen -dmS containerd bash -c "containerd > >(tee -a /tmp/vhive-logs/containerd.stdout) 2> >(tee -a /tmp/vhive-logs/containerd.stderr >&2)"
    ```
    > **Note:**
    >
    > `screen` is a terminal multiplexer similar to `tmux` but widely available by default.
    >
    > Starting long-running daemons in the background using `screen` allows you to use a single
    > terminal (an SSH session most likely) by keeping it unoccupied and ensures that
    > daemons will not be terminated when you logout (voluntarily, or because of connection issues).
    >
    > - To (re-)attach a background terminal:
    >   ```bash
    >   sudo screen -rd <name>
    >   ```
    >  - To detach (from an attached terminal):\
    >    <kbd>Ctrl</kbd>+<kbd>A</kbd> then <kbd>D</kbd>
    > - To kill a background terminal:
    >   ```bash
    >   sudo screen -XS <name> quit
    >   ```
    > - To list all the sessions:
    >   ```bash
    >   sudo screen -ls
    >   ```
3. Start `firecracker-containerd` in a background terminal named `firecracker`:
    ```bash
    sudo PATH=$PATH screen -dmS firecracker bash -c "/usr/local/bin/firecracker-containerd --config /etc/firecracker-containerd/config.toml > >(tee -a /tmp/vhive-logs/firecracker.stdout) 2> >(tee -a /tmp/vhive-logs/firecracker.stderr >&2)"
    ```

4. Build vHive host orchestrator:
    ```bash
    source /etc/profile && go build
    ```
5. Start `vHive` in a background terminal named `vhive`:
    ```bash
    # EITHER
    sudo screen -dmS vhive bash -c "./vhive > >(tee -a /tmp/vhive-logs/vhive.stdout) 2> >(tee -a /tmp/vhive-logs/vhive.stderr >&2)"
    # OR
    sudo screen -dmS vhive bash -c "./vhive -snapshots > >(tee -a /tmp/vhive-logs/vhive.stdout) 2> >(tee -a /tmp/vhive-logs/vhive.stderr >&2)"
    # OR
    sudo screen -dmS vhive bash -c "./vhive -snapshots -upf > >(tee -a /tmp/vhive-logs/vhive.stdout) 2> >(tee -a /tmp/vhive-logs/vhive.stderr >&2)"
    ```
    > **Note:**
    >
    > By default, the microVMs are booted, `-snapshots` enables snapshots after the 2nd invocation of each function.
    >
    > If `-snapshots` and `-upf` are specified, the snapshots are accelerated with the Record-and-Prefetch (REAP) technique that we described in our ASPLOS'21 paper ([extended abstract][ext-abstract], [full paper](papers/REAP_ASPLOS21.pdf)).

### 3. Configure Master Node
**On the master node**, execute the following instructions below **as a non-root user with sudo rights** using **bash**:
1. Start `containerd` in a background terminal named `containerd`:
    ```bash
    sudo screen -dmS containerd bash -c "containerd > >(tee -a /tmp/vhive-logs/containerd.stdout) 2> >(tee -a /tmp/vhive-logs/containerd.stderr >&2)"
    ```
2. Run the script that creates the multinode cluster (without `stargz`):
    ```bash
    ./scripts/cluster/create_multinode_cluster.sh > >(tee -a /tmp/vhive-logs/create_multinode_cluster.stdout) 2> >(tee -a /tmp/vhive-logs/create_multinode_cluster.stderr >&2)
    ```
    > **BEWARE:**
    >
    > The script will ask you the following:
    > ```
    > All nodes need to be joined in the cluster. Have you joined all nodes? (y/n)
    > ```
    > **Leave this hanging in the terminal as we will go back to this later.**
    >
    > However, in the same terminal you will see a command in following format:
    > ```
    > kubeadm join 128.110.154.221:6443 --token <token> \
    >     --discovery-token-ca-cert-hash sha256:<hash>
    > ```
    > Please copy the both lines of this command.

    > **IMPORTANT:**
    > If you built the cluster using the `stock-only` flag, execute the following 
    > script instead:
    >   ```bash
    >   ./scripts/cluster/create_multinode_cluster.sh stock-only > >(tee -a /tmp/vhive-logs/
    > create_multinode_cluster.stdout) 2> >(tee -a /tmp/vhive-logs/create_multinode_cluster.stderr >&2)
    >   ```

### 4. Configure Worker Nodes
**On each worker node**, execute the following instructions below **as a non-root user with sudo rights** using **bash**:

1. Add the current worker to the Kubernetes cluster, by executing the command you have copied in step (3.2) **using sudo**:
    ```bash
    sudo kubeadm join IP:PORT --token <token> --discovery-token-ca-cert-hash sha256:<hash> > >(tee -a /tmp/vhive-logs/kubeadm_join.stdout) 2> >(tee -a /tmp/vhive-logs/kubeadm_join.stderr >&2)
    ```
    > **Note:**
    >
    > On success, you should see the following message:
    > ```
    > This node has joined the cluster:
    > * Certificate signing request was sent to apiserver and a response was received.
    > * The Kubelet was informed of the new secure connection details.
    > ```

### 5. Finalise Master Node
**On the master node**, execute the following instructions below **as a non-root user with sudo rights** using **bash**:

1. As all worker nodes have been joined, and answer with `y` to the prompt we have left hanging in the terminal.
2. As the cluster is setting up now, wait until all pods show as `Running` or `Completed`:
    ```bash
    watch kubectl get pods --all-namespaces
    ```

**Congrats, your Knative cluster is ready!**


## III. Setup a Single-Node Cluster
### 1. Manual
In essence, you will execute the same commands for master and worker setups but on a single node.

5 seconds delay has been added between the commands to ensure that components have enough time to initialize.

Execute the following below **as a non-root user with sudo rights** using **bash**:
1. Run the node setup script:
    ```bash
    ./scripts/cloudlab/setup_node.sh;
    ```
    > **Note:**
    > To enable runs with `stargz` images, setup kubelet by adding the `stock-only` and `use-stargz`
    > flags as follows:
    >   ```bash
    >   ./scripts/cloudlab/setup_node.sh stock-only use-stargz
    >   ```
    > **IMPORTANT**
    > Currently `stargz` is only supported in native kubelet contexts without firecracker. 
    > Therefore, the following steps from this guide must **not** be executed:
    > * `2.3`,
    > * `2.4`,
    > * `2.5`.
2. Start `containerd` in a background terminal named `containerd`:
    ```bash
    sudo screen -dmS containerd containerd; sleep 5;
    ```
    > **Note:**
    >
    > Regarding `screen` and starting daemons in background terminals, see the note
    > in step 2 of subsection II.2 _Setup Worker Nodes_.
3. Start `firecracker-containerd` in a background named `firecracker`:
    ```bash
    sudo PATH=$PATH screen -dmS firecracker /usr/local/bin/firecracker-containerd --config /etc/firecracker-containerd/config.toml; sleep 5;
    ```
4. Build vHive host orchestrator:
    ```bash
    source /etc/profile && go build;
    ```
5. Start `vHive` in a background terminal named `vhive`:
    ```bash
    sudo screen -dmS vhive ./vhive; sleep 5;
    ```
6. Run the single node cluster setup script:
    ```bash
    ./scripts/cluster/create_one_node_cluster.sh
    ```
    > **IMPORTANT:**
    > If you setup the node using the `stock-only` flag, execute the following 
    > script instead:
    >   ```bash
    >   ./scripts/cluster/create_one_node_cluster.sh stock-only
    >   ```

### 2. Clean Up
```bash
./scripts/github_runner/clean_cri_runner.sh
```

### 3. Using a Script
This script stops the existing cluster if any, cleans up and then starts a fresh single-node cluster.

```bash
export GITHUB_VHIVE_ARGS="[-dbg] [-snapshots] [-upf]" # specify if to enable debug logs; cold starts: snapshots, REAP snapshots (optional)
scripts/cloudlab/start_onenode_vhive_cluster.sh
```

## IV. Deploying and Invoking Functions in vHive
This section is only for synchronous (i.e., Knative Serving) functions. Please refer to
[Adding Benchmarks to vHive/Knative and Stock Knative][kn-benchmark]
for benchmarking asynchronous (i.e., Knative Eventing) case and more details about both.

### 1. Deploy Functions
**On the master node**, execute the following instructions below using **bash**:

1. Optionally, configure the types and the number of functions to deploy in `examples/deployer/functions.json`.
2. Run the deployer client:
    ```bash
    source /etc/profile && pushd ./examples/deployer && go build && popd && ./examples/deployer/deployer
    ```
    > **BEWARE:**
    >
    > Deployer **cannot be used for Knative eventing** (i.e., asynchronous) workflows. You need to deploy them manually instead.
    >
    > Deployer uses YAML files defined in configs/knative-workload that are **specific** to firecracker. Please refer to the [Developer's Guide](developers_guide.md) for deploying functions in the container or gVisor based environment.

    > **Note:**
    >
    > There are runtime arguments that you can specify if necessary.
    >
    > The script writes the deployed functions' endpoints in a file (`endpoints.json` by default).

### 2. Invoke Functions
**On any node**, execute the following instructions below using **bash**:

1. Run the invoker client:
    ```bash
    pushd ./examples/invoker && go build && popd && ./examples/invoker/invoker
    ```

    > **Note:**
    >
    > In order to run the invoker client on another node, copy the `endpoints.json` file to the target node and run the invoker, specifying the path to the file as `-endpointsFile path/to/endpoints.json`.
    >
    > There are runtime arguments (e.g., RPS or requests-per-second target, experiment duration) that you can specify if necessary.
    >
    > After invoking the functions from the input file (`endpoints.json` by default), the script writes the measured latencies to an output file (`rps<RPS>_lat.csv` by default, where `<RPS>` is the observed requests-per-sec value) for further analysis.

### 3. Delete Deployed Functions
**On the master node**, execute the following instructions below using **bash**:

1. Delete **all** deployed functions:
    ```bash
    kn service delete --all
    ```

## V. Deploying eStargz-based Functions
This section provides an example function run using a `nodejs` base image that has been converted to the `stargz` format. To create other images supported by `stargz`, please refer to the [creating-estargz-images-using-ctr-remote](https://github.com/containerd/stargz-snapshotter/tree/cmd/v0.12.1#creating-estargz-images-using-ctr-remote) section of the official `stargz` repository.

Our example image can be found in [/configs/knative_workloads/stargz-node.yaml](../configs/knative_workloads/stargz-node.yaml) and can be run with:
```bash
kn service apply stargz-test -f configs/knative_workloads/stargz-node.yaml --concurrency-target 1
```
### 1. Deploy and Invoke Functions
**On the master node**, execute the following using **bash**:

```bash
kn service apply <name> -f <yaml_config_path> --concurrency-target 1
```

Interact with the deployed function from any node using the exposed interface of the deployed function.
### 2. Delete Deployed Function
**On the master node**, execute the following using **bash**:

1. Delete **all** deployed functions:
    ```bash
    kn service delete --all
    ```

[github-toc]: https://github.blog/changelog/2021-04-13-table-of-contents-support-in-markdown-files/
[cloudlab]: https://www.cloudlab.us
[cloudlab-pf]: https://www.cloudlab.us/p/faas-sched/vhive-ubuntu20
[cloudlab-hw]: https://docs.cloudlab.us/hardware.html
[ext-abstract]: https://asplos-conference.org/abstracts/asplos21-paper212-extended_abstract.pdf
[kn-benchmark]: https://github.com/vhive-serverless/vSwarm/blob/main/docs/adding_benchmarks.md
