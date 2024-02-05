# K8s Power Manager

## Components
1. **PowerManager Controller**: ensures the actual state matches the desired state of the cluster.
2. **PowerConfig Controller**: sees the powerConfig created by user and deploys Power Node Agents onto each node specified using a DaemonSet.
    - powerNodeSelector: A key/value map used to define a list of node labels that a node must satisfy for the operator's node
      agent to be deployed.
    - powerProfiles: The list of PowerProfiles that the user wants available on the nodes
3. **Power Node Agent**: containerized applications used to communicate with the node's Kubelet PodResources endpoint to discover the exact CPUs that
   are allocated per container and tune frequency of the cores as requested


## Setup
### 1. Manual
#### on both nodes
    git clone -b new_test --depth=1 https://github.com/vhive-serverless/vhive.git
    cd vhive
    mkdir -p /tmp/vhive-logs
    ./scripts/cloudlab/setup_node.sh stock-only > >(tee -a /tmp/vhive-logs/setup_node.stdout) 2> >(tee -a /tmp/vhive-logs/setup_node.stderr >&2)

#### for worker
    ./scripts/cluster/setup_worker_kubelet.sh stock-only > >(tee -a /tmp/vhive-logs/setup_worker_kubelet.stdout) 2> >(tee -a /tmp/vhive-logs/setup_worker_kubelet.stderr >&2)
    sudo screen -dmS containerd bash -c "containerd > >(tee -a /tmp/vhive-logs/containerd.stdout) 2> >(tee -a /tmp/vhive-logs/containerd.stderr >&2)"

#### for master
    sudo screen -dmS containerd bash -c "containerd > >(tee -a /tmp/vhive-logs/containerd.stdout) 2> >(tee -a /tmp/vhive-logs/containerd.stderr >&2)"
    ./scripts/cluster/create_multinode_cluster.sh stock-only > >(tee -a /tmp/vhive-logs/create_multinode_cluster.stdout) 2> >(tee -a /tmp/vhive-logs/create_multinode_cluster.stderr >&2)

 join the cluster from worker, answer 'y' to master

## Setup 
### 1. Manual

Execute the following below **as a non-root user with sudo rights** using **bash**:
1. On master node, run the node setup script:
    ```bash
    ./examples/powermanger/setup_power_manager.sh;
    go run ./examples/powermanger/experiment_2.go
    ```
2. On worker node, run:
    ```bash
    ./examples/powermanger/setup_power_manager.sh;
    go run ./examples/powermanger/experiment_2.go
    ```    
    while true; do current_time=$(date -u +%s%3N); cpu_average=$(grep 'MHz' /proc/cpuinfo | awk '{ total += $4 } END { print total / NR }'); echo "${current_time}: ${cpu_average}" >> node1_freq.txt; sleep 0.01; done

    sudo turbostat --quiet --interval 1 --show PkgWatt | while read line; do if [[ ! $line =~ PkgWatt ]]; then timestamp=$(date +%s%3N); echo "$timestamp: $line"; fi; done >> node1_power.txt
    ```

2. Clean Up
   ```bash
   ./scripts/github_runner/clean_cri_runner.sh
   ```