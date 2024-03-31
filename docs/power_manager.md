# K8s Power Manager

## Components
1. **Power Manager Controller**: ensures the actual state matches the desired state of the cluster.
2. **Power Config Controller**: sees the power config created by user and deploys Power Node Agents onto each node specified using a daemon set.
    - power node selector: A key/value map used to define a list of node labels that a node must satisfy for the operator's node
      agent to be deployed.
    - power profiles: The list of power profiles that the user wants available on the nodes
3. **Power Node Agent**: containerized applications used to communicate with the node's Kubelet pod resources endpoint to discover the exact CPUs that
   are allocated per container and tune frequency of the cores as requested

4. **Power Profile**: predefined configuration that specifies how the system should manage power consumption for various components such as CPUs. It includes settings applied to host level such as CPU frequency, governor etc.

4. **Power Workload**: the object used to define the lists of CPUs configured with a particular Power Profile. A power workload is created for each Power Profile on each Node with the Power Node Agent deployed. A power workload is represented in the Intel Power Optimization Library by a Pool. The Pools hold the values of the Power Profile used, their frequencies, and the CPUs that need to be configured. The creation of the Pool – and any additions to the Pool – then 
carries out the changes.

## Setup 
Execute the following below **as a non-root user with sudo rights** using **bash**:
1. Follow [a quick-start guide](quickstart_guide.md) to set up a Knative cluster.
2. On master node, export NODE_NAME to your node name and run K8s power manager set up script:
  ```bash
    export NODE_NAME= *Name of the node you want to apply shared profile on*
    ./scripts/power_manager/setup_power_manager.sh;
  ```

  This will install and configure the Kubernetes Power Manager for managing power consumption in a Kubernetes cluster. It clones the Power Manager repository, sets up the necessary namespace, service account, and Role-based Access Control rule, then generates and installs custom resource definitions, and deploys the Power Manager controller. It also applies a Power config to manage the power node agents, a shared profile for specifying CPU frequencies, and a shared workload for applying the CPU tuning settings. 
