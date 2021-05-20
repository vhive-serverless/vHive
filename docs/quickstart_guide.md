# vHive setup guidelines

This wiki page describes how to set up vHive serverless cluster.
These guide describes the instructions for setting up an N-node serverless cluster.
We tested these instructions by setting up a **2-node** cluster on [CloudLab](https://www.cloudlab.us),
using any of the following machines: xl170 on Utah, rs440 on Mass, m400 on OneLab.
The users can consider other SSD-based machines.

## Host platform requirements

### Hardware

1. Two x86 servers in the same network. We have not tried vHive with Arm but it may not be hard to port because Firecracker supports Arm64 ISA.

2. Hardware support for virtualization and KVM. Note: Nested virtualization is supported provided that KVM is available.

3. The root partition of the host filesystem should be mounted on an **SSD**. that is critical for snapshot-based cold-starts.
We expect vHive to work on machines that use HDDs but there could be timeout-related issues with large Docker images (>1GB).


### Software
1. Ubuntu/Debian with root access and `apt` package manager on the host (tested on Ubuntu 18.04, v4.15).
Other OS-es should work too but they require changes in our setup scripts.

2. Passwordless SSH. Copy the SSH keys that you use to authenticate on GitHub to all the nodes and
type `eval "$(ssh-agent -s)" && ssh-add` to allow ssh authentication in the background.


## Set up serverless (Knative) cluster

### On each node

1. Clone the vHive repository
`git clone https://github.com/ease-lab/vhive.git`

Assuming you are at the root of the repository:

2. Run `./scripts/cloudlab/setup_node.sh`. This script can print `Command failed` when creating the devmapper. This can be safely ignored.


### On each worker node

1. Run `./scripts/cluster/setup_worker_kubelet.sh`

2. In a new terminal, start containerd `sudo containerd`. This can show an error `failed to load cni during init`. We will initialize CNI later.

3. In a new terminal, start firecracker-containerd:

```
sudo PATH=$PATH /usr/local/bin/firecracker-containerd --config /etc/firecracker-containerd/config.toml
```

4. Build and start the vHive host orchestrator.
By default, the microVMs are booted, `snapshots` enables snapshots after the 2nd invocation of each function.
If `-snapshots` and `-upf` are specified, the snapshots are accelerated with the Record-and-Prefetch (REAP)
technique that we described in our ASPLOS'21 paper
([extended abstract](https://asplos-conference.org/abstracts/asplos21-paper212-extended_abstract.pdf),
[full paper](papers/REAP_ASPLOS21.pdf)).

```
source /etc/profile && go build
sudo ./vhive [-snapshots|-snapshots -upf]
```
**Note:** it is important to build the code before running tests.


### On the master, **once all workers are all set**

1. Start containerd `sudo containerd` in a new terminal. This can show an error `failed to load cni during init`. We will initialize CNI later.

2. Run `./scripts/cluster/create_multinode_cluster.sh`. This will ask you the following: `All nodes need to be joined in the cluster.
Have you joined all nodes? (y/n)` Leave this hanging in the terminal as we will go back to this later.
However, in the same terminal you will see a line like the following: `kubeadm join IP:PORT --token <token> --discovery-token-ca-cert-hash <hash>`.
You need to use this command to add each of the worker nodes to the Kubernetes cluster in the next step below.

### On each worker node

Add all worker nodes to the Kubernetes cluster by typing the command **on each worker** given to you on the master node:
```
sudo kubeadm join <IP>:<PORT> --token <token> --discovery-token-ca-cert-hash <hash>
```

### On the master node

1. Once all worker nodes are joined, go back to the master and answer with "y" to the prompt. The cluster is setting up now.

2. Once the last command returns, run `watch kubectl get pods --all-namespaces` and wait until all pods show as "Running" or "Completed".

Congrats, your Knative cluster is ready!


## Deploy functions
You can configure the types and the number of functions to deploy in `examples/deployer/functions.json`. Then, type on the master node:

```
go run examples/deployer/client.go
```

Note that there are runtime arguments that you can specify if necessary. The script writes the deployed functions' URLs in a file (`urls.txt` by default).

## Invoke functions
To invoke the functions by typing (on any node of the Kubernetes cluster):

```
go run examples/invoker/client.go
```

Note that there are runtime arguments (e.g., RPS or requests-per-second target, experiment duration) that you can specify if necessary.
After invoking the functions from the input file (urls.txt by default), the script writes the measured latencies to an output file
(rps<RPS>_lat.csv` by default, where <RPS> is the observed requests-per-sec value) for further analysis.

## Delete deployed functions

Type on the master node:
```
kn service delete --all
```

## Setup a single-node cluster (master and worker functionality on the same node)

### Manual
Start each component separately.

```bash
./scripts/cloudlab/setup_node.sh
sudo containerd
sudo PATH=$PATH /usr/local/bin/firecracker-containerd --config /etc/firecracker-containerd/config.toml
source /etc/profile && go build && sudo ./vhive
./scripts/cluster/create_one_node_cluster.sh
```

### Clean up
```bash
./scripts/github_runner/clean_cri_runner.sh
```

### Using a script
This script stops the existing cluster if any, cleans up and then starts a fresh single-node cluster.

```bash
export GITHUB_VHIVE_ARGS="[-dbg] [-snapshots] [-upf]" # specify if to enable debug logs; cold starts: snapshots, REAP snapshots (optional)
scripts/cloudlab/start_onenode_vhive_cluster.sh
```

## CloudLab deployment notes

### CloudLab Profile

It is recommended to use a base Ubuntu 18.04 image for each node and connect the nodes in a LAN.
You can use our CloudLab profile that is called [RPerf/vHive-cluster-env](https://www.cloudlab.us/p/RPerf/vHive-cluster-env).

### Nodes to Rent
SSD-equipped nodes are highly recommended. We suggest renting nodes on CloudLab as their service is available to researchers world-wide. Full list of CloudLab nodes can be found [here](https://docs.cloudlab.us/hardware.html).

List of tested nodes on CloudLab: xl170 on Utah, rs440 on Mass, m400 on OneLab. All of these machines are SSD-equipped. The xl170 are normally less occupied than the other two.
