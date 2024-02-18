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

Execute the following below **as a non-root user with sudo rights** using **bash**:
1. On master node, run the node setup script:
    ```bash
    ./examples/powermanger/setup_power_manager.sh;
    ```
2. On worker node, run:
    ```bash
    go run ./examples/powermanger/workload_sensitivity_exp.go
    ```

2. Clean Up
   ```bash
   ./scripts/github_runner/clean_cri_runner.sh
   ```