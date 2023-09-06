# K8s Power Manager

## Components
1. **PowerManager Controller**: ensures the actual state matches the desired state of the cluster. 
2. **PowerConfig Controller**: sees the powerConfig created by user and deploys Power Node Agents onto each node specified using a DaemonSet.
   - powerNodeSelector: A key/value map used to define a list of node labels that a node must satisfy for the operator’s node
     agent to be deployed.
   - powerProfiles: The list of PowerProfiles that the user wants available on the nodes
3. **Power Node Agent**: containerized applications used to communicate with the node’s Kubelet PodResources endpoint to discover the exact CPUs that
    are allocated per container and tune frequency of the cores as requested


## Setup 
### 1. Manual

Execute the following below **as a non-root user with sudo rights** using **bash**:
1. Run the node setup script:
    ```bash
   git clone -b power_manager --depth=1 https://github.com/vhive-serverless/vhive.git
    ./scripts/power_manager/setup_poer_manager.sh;
    