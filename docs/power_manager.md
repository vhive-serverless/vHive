# K8s Power Manager

## Components
1. **PowerManager Controller**: ensures the actual state matches the desired state of the cluster.
2. **PowerConfig Controller**: sees the powerConfig created by user and deploys Power Node Agents onto each node specified using a DaemonSet.
    - powerNodeSelector: A key/value map used to define a list of node labels that a node must satisfy for the operator's node
      agent to be deployed.
    - powerProfiles: The list of PowerProfiles that the user wants available on the nodes
3. **Power Node Agent**: containerized applications used to communicate with the node's Kubelet PodResources endpoint to discover the exact CPUs that
   are allocated per container and tune frequency of the cores as requested

4. **Power Profile**: predefined configuration that specifies how the system should manage power consumption for various components such as CPUs and GPUs. It includes settings applied to host level such as CPU frequency, governer etc.

4. **Power Workload**: the object used to define the lists of CPUs configured with a particular PowerProfile. A PowerWorkload is created for each PowerProfile on each Node with the Power Node Agent deployed. A PowerWorkload is represented in the Intel Power Optimization Library by a Pool. The Pools hold the values of the PowerProfile used, their frequencies, and the CPUs that need to be configured. The creation of the Pool – and any additions to the Pool – then 
carries out the changes.

## Setup 

Execute the following below **as a non-root user with sudo rights** using **bash**:
1. Follow [a quick-start guide](quickstart_guide.md) to set up a Knative cluster to run the experiments. 

2. 4 vSwarm benchmarks are used to run the experiments (Spinning, Sleeping, AES, Auth). On master node, deploy these benchmarks on the Knative cluster.
    ```bash
    git clone --depth=1 https://github.com/vhive-serverless/vSwarm.git

    cd $HOME/vSwarm/tools/test-client && go build ./test-client.go

    kn service apply -f $HOME/vSwarm/benchmarks/auth/yamls/knative/kn-auth-python.yaml
    kn service apply -f $HOME/vSwarm/benchmarks/aes/yamls/knative/kn-aes-python.yaml
    kn service apply -f $HOME/vSwarm/benchmarks/sleeping/yamls/knative/kn-sleeping-go.yaml
    kn service apply -f $HOME/vSwarm/benchmarks/spinning/yamls/knative/kn-spinning-go.yaml
    ```

### Experiment 1: Workload sensitivity 
2 node cluster is needed.
1. On master node, run the node setup script:
    ```bash
    ./scripts/power_manager/setup_power_manager.sh;
    ```
   Then run the experiment:
    ```bash
    go run $HOME/vhive/examples/power_manager/workload_sensitivity_exp/main.go;
    ```

### Experiment 2: Internode scaling
3 node cluster is needed. 3 scenarios are performed:
- Scenario 1: All worker nodes have low frequency 
- Scenario 2: All worker nodes have high frequency
- Scenario 3: 1 worker node has high frequency, another with low frequency (need to manually tune like experiment 3 point 4&5 below)

1. On master node, run the node setup script:
    ```bash
    ./scripts/power_manager/setup_power_manager.sh;
    ```
   Then run the experiment:
    ```bash
    go run $HOME/vhive/examples/power_manager/internode_scaling_exp/main.go;
    ```
 
### Experiment 3: Class Assignment 
3 node cluster is needed. We need to able to assign the workload to specific node.

1. Thus on master node, we need to enable nodeSelector:
```bash
   kubectl patch configmap config-features -n knative-serving -p '{"data": {"kubernetes.podspec-nodeselector": "enabled"}}'
```

2. We need to modify these benchmark knative yaml file to add nodeSelector in vSwarm before deployment (Spinning and AES to worker-high class, Sleeping and Auth to worker-low class). For example: 
````yaml
apiVersion: serving.knative.dev/v1
kind: Service
metadata:
 name: spinning-go
 namespace: default
spec:
 template:
   spec:
     nodeSelector:
       loader-nodetype: worker-high
     containers:
       - image: docker.io/vhiveease/relay:latest
         ports:
           - name: h2c
             containerPort: 50000
         args:
           - --addr=0.0.0.0:50000
           - --function-endpoint-url=0.0.0.0
           - --function-endpoint-port=50051
           - --function-name=aes-go
       - image: docker.io/kt05docker/sleeping-go:latest
         args:
           - --addr=0.0.0.0:50051    - 
```` 

3. On master node, label the worker node 
    ```bash
    kubectl label node node-1.kt-cluster.ntu-cloud-pg0.utah.cloudlab.us loader-nodetype=worker-low
    kubectl label node node-2.kt-cluster.ntu-cloud-pg0.utah.cloudlab.us loader-nodetype=worker-high 
    ```
    Run the node setup script:
    ```bash
    ./scripts/power_manager/setup_power_manager.sh;
    ```
4. On worker node 1, manually set all CPU frequency to 1.2GHz. ie run the below command for all CPU core:
    ```bash
    echo performance | sudo tee /sys/devices/system/cpu/cpu0/cpufreq/scaling_governor
    echo 1200000 | sudo tee /sys/devices/system/cpu/cpu0/cpufreq/scaling_min_freq
    echo 1200000 | sudo tee /sys/devices/system/cpu/cpu0/cpufreq/scaling_max_freq
    ```
5. On worker node 2, manually set all CPU frequency to 2.4GHz.
    ```bash
    echo performance | sudo tee /sys/devices/system/cpu/cpu0/cpufreq/scaling_governor
    echo 2400000 | sudo tee /sys/devices/system/cpu/cpu0/cpufreq/scaling_min_freq
    echo 2400000 | sudo tee /sys/devices/system/cpu/cpu0/cpufreq/scaling_max_freq
    ```

6. Run the experiment:
    ```bash
    go run $HOME/vhive/examples/power_manager/assign_exp/main.go;
    ```