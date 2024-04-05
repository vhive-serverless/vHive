# K8s Power Manager Experiments

## Setup 
1. 4 vSwarm benchmarks are used to run the experiments. On master node, deploy these benchmarks on the Knative cluster.
    ```bash
    git clone --depth=1 https://github.com/vhive-serverless/vSwarm.git

    cd $HOME/vSwarm/tools/test-client && go build ./test-client.go

    kn service apply -f $HOME/vSwarm/benchmarks/auth/yamls/knative/kn-auth-python.yaml
    kn service apply -f $HOME/vSwarm/benchmarks/aes/yamls/knative/kn-aes-python.yaml
    kn service apply -f $HOME/vSwarm/benchmarks/sleeping/yamls/knative/kn-sleeping-go.yaml
    kn service apply -f $HOME/vSwarm/benchmarks/spinning/yamls/knative/kn-spinning-go.yaml
    ```
2. Change the global variable node names in power_manager/util.go based on the actual names of your node.

### Experiment 1: Workload sensitivity 
This experiment is to confirm that workload sensitivity to CPU frequency varies for different types of workloads, with CPU-bound workloads showing greater sensitivity than I/O-bound workloads as I/O-bound workloads are primarily limited by factors such as disk and network speed rather than CPU processing speed. 2 node knative cluster is needed for this experiment.

1. On master node, run the node setup script:
    ```bash
    ./scripts/power_manager/setup_power_manager.sh;
    ```
   Then run the experiment:
    ```bash
    go run $HOME/vhive/examples/power_manager/workload_sensitivity/main.go;
    ```

### Experiment 2: Internode scaling
3 node cluster is needed. 3 scenarios are performed:
- Scenario 1: All worker nodes have low frequency 
- Scenario 2: All worker nodes have high frequency
- Scenario 3: 1 worker node has high frequency, another with low frequency (need to manually tune like experiment 3 point 4&5 below)

This experiment is to confirm that using all low-frequency combinations results in low power consumption but comes with the drawback of high latency. Conversely, opting for all high-frequency combinations maximizes performance by significantly reducing latency but it does so at the cost of high-power consumption. A 50/50 mix of frequencies strikes a
balance, offering medium power consumption with the benefit of low latency.

1. On master node, run the node setup script:
    ```bash
    ./scripts/power_manager/setup_power_manager.sh;
    ```
   Then run the experiment:
    ```bash
    go run $HOME/vhive/examples/power_manager/internode_scaling/main.go;
    ```

### Experiment 3: Class Assignment 
3 node cluster is needed with 1 master node, 1 high frequency worker node and 1 low frequency worker node (manually set up as experiment 2 scenario 3).

This experiment is to confirm that the automatic assignment of workloads based on their workload sensitivity will lead to improved performance and optimized power consumption. Sensitive workloads with more than a 40% latency difference between 5th and 90th percentiles (such as Spinning) will be automatically assigned to high frequency node should experience lower latency, while less sensitive workloads (such as Sleeping) can be efficiently handled by low-frequency nodes, conserving energy without significantly impacting performance.

1. Thus on master node, we need to enable nodeSelector:
```bash
   kubectl patch configmap config-features -n knative-serving -p '{"data": {"kubernetes.podspec-nodeselector": "enabled"}}'
```

2. On master node, label the worker node 
    ```bash
    kubectl label node node-1.kt-cluster.ntu-cloud-pg0.utah.cloudlab.us loader-nodetype=worker-low
    kubectl label node node-2.kt-cluster.ntu-cloud-pg0.utah.cloudlab.us loader-nodetype=worker-high 
    ```
    Run the node setup script:
    ```bash
    ./scripts/power_manager/setup_power_manager.sh;
    ```
4. On worker node 1, manually set all CPU frequency to 1.2GHz. i.e. run the below command for all CPU core:
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
    go run $HOME/vhive/examples/power_manager/assignment/main.go;
    ```
