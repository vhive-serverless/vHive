# EasyOpenYurt

## 1. Introduction

[OpenYurt](https://github.com/openyurtio/openyurt) is built based on upstream Kubernetes and has been designed to meet various DevOps requirements against typical edge infrastructures.

This program can help you to set up an OpenYurt cluster quickly and easily for development and test. It currently supports two main usages:

1. **From scratch**: Firstly set up a Kubernetes cluster using kubeadm and then deploy OpenYurt on it.
2. **Based on existing Kubernetes cluster**: Deploy OpenYurt directly on an existing Kubernetes cluster.

Additionally, several YAML template files which basically shows how to deploy services on OpenYurt are provided along with the program.

**Currently supported and tested platforms:**

|      OS      | ARCH  |
| :----------: | :---: |
| Ubuntu 22.04 | amd64 |
| Ubuntu 20.04 | amd64 |

**Currently supported and tested Shells:** `zsh`, `bash`

**<u>Warning:</u>** <u>This is an experimental program under development, **DO NOT** attempt to use it in production environment! Back up your system in advance to avoid possible damage.</u>

Finally, the program is well commented. You can look at the source and see what it is going to do before running. Have a good day!

## 2. Usage

**General Usage:**

```bash
./easy_openyurt <object: system | kube | yurt> <nodeRole: master | worker> <operation: init | join | expand> [Parameters...]
```

By default, **logs will be written into two files**: `easyOpenYurtCommon.log` and `easyOpenYurtError.log` **in the current directory**.

### 2.1 Get easy_openyurt

**You can either download the easy_openyurt binary file directly or build it from source**:

#### 2.1.1 Download the binary file directly

Go for [releases](https://github.com/flyinghorse0510/easy_openyurt/releases) and download the appropriate binary version.

#### 2.1.2 Build from source

**Building from source requires Golang(version at least 1.20) installed.**

##### Build for current system
```bash
git clone https://github.com/flyinghorse0510/easy_openyurt.git
cd easy_openyurt/
go build -o easy_openyurt ./src/easy_openyurt/*.go
```

##### Build for all targets
```bash
git clone https://github.com/flyinghorse0510/easy_openyurt.git
cd easy_openyurt/src/easy_openyurt/
chmod +x ./build.sh && ./build.sh
```
Compiled executable files will be in the `bin` directory.

### 2.2 Configure System on Master / Worker Node

> If you already have an existing kubernetes cluster, you can directly go to [2.4 Deploy OpenYurt on Kubernetes Cluster](#24-deploy-openyurt-on-kubernetes-cluster)

This procedure will install and configure required components in your system, such as:

- `containerd`
- `runc`
- `golang`
- `kubeadm`, `kubectl`, `kubelet`
- ……

To initialize your system, use the following command:

```bash
./easy_openyurt system master init # on the master node
./easy_openyurt system worker init # on the worker node
```

Additionally, if you want to change the version of components to be installed, you can add extra optional parameters behind(or add -h for help):

```bash
./easy_openyurt system master init -h
#### Output ####
# Usage of ./easy_openyurt system master init:
#   -cni-plugins-version string
#         CNI plugins version (default "1.2.0")
#   -containerd-version string
#         Containerd version (default "1.6.18")
#   -go-version string
#         Golang version (default "1.18.10")
#   -h    Show help
#   -help
#         Show help
#   -kubeadm-version string
#         Kubeadm version (default "1.25.9-00")
#   -kubectl-version string
#         Kubectl version (default "1.25.9-00")
#   -kubelet-version string
#         Kubelet version (default "1.25.9-00")
#   -runc-version string
#         Runc version (default "1.1.4")
```

### 2.3 Set up Kubernetes Cluster

#### 2.3.1 Set up Master Node

On master node, use the following command:

```bash
./easy_openyurt kube master init
```

By default, [the `kubeadm` uses the network interface associated with the default gateway to set the advertise address for this particular control-plane(master) node's API server](https://kubernetes.io/docs/setup/production-environment/tools/kubeadm/create-cluster-kubeadm/). To use a different network interface, provide an extra parameter to the program:

```bash
./easy_openyurt kube master init -apiserver-advertise-address [apiserverAdvertiseAddress]
# For example:
./easy_openyurt kube master init -apiserver-advertise-address 192.168.18.2
```

If everything goes well, you can find one file called `masterKey.yaml` in the current directory, which includes information that can be subsequently used to set up the worker node in Kubernetes cluster:

```yaml
# Content Template of `masterKey.yaml`
apiserverAdvertiseAddress: xxx.xxx.xxx.xxx
apiserverPort: xxxx
apiserverToken: xxxxxxxxxx
apiserverTokenHash: sha256:xxxxxxxxxx
```

To view the help and all available optional parameters, add `-h` to see more details:

```bash
./easy_openyurt kube master init -h
#### Output ####
# Usage of ./easy_openyurt kube master init:
#   -alternative-image-repo string
#         Alternative image repository
#   -apiserver-advertise-address string
#         Kubernetes API server advertise address
#   -h    Show help
#   -help
#         Show help
#   -k8s-version string
#         Kubernetes version (default "1.25.9")
```

#### 2.3.2 Set up Worker Node

On worker node, to join the Kubernetes cluster, use the following command:

```bash
./easy_openyurt kube worker join -apiserver-advertise-address <apiserverAdvertiseAddress> -apiserver-token <apiserverToken> -apiserver-token-hash <apiserverTokenHash>
# You can find these parameters in file `masterKey.yaml` previously introduced on the master node
# For Example:
./easy_openyurt kube worker join -apiserver-advertise-address 192.168.18.2 -apiserver-token xxxxxxxxxx -apiserver-token-hash sha256:xxxxxxxxxx
```

To view the help and all available optional parameters, add `-h` to see more details:

```bash
./easy_openyurt kube worker join -h
#### Output ####
# Usage of ./easy_openyurt kube worker join:
#   -apiserver-advertise-address string
#         Kubernetes API server advertise address (**REQUIRED**)
#   -apiserver-port string
#         Kubernetes API server port (default "6443")
#   -apiserver-token string
#         Kubernetes API server token (**REQUIRED**)
#   -apiserver-token-hash string
#         Kubernetes API server token hash (**REQUIRED**)
#   -h    Show help
#   -help
#         Show help
```

### 2.4 Deploy OpenYurt on Kubernetes Cluster
> If you just want a vanilla Kubernetes cluster(or vanilla Knative/vHive furthermore), please just skip this section.

#### 2.4.1 Deploy on Master Node

On master node, to deploy OpenYurt, use the following command:

```bash
./easy_openyurt yurt master init
```

To view the help and all available optional parameters, add `-h` to see more details:

```bash
./easy_openyurt yurt master init -h
#### Output ####
# Usage of ./easy_openyurt yurt master init:
#   -h    Show help
#   -help
#         Show help
#   -master-as-cloud
#         Treat master as cloud node (default true)
```

#### 2.4.2 Deploy on Worker Node

**<u>Warning:</u>** <u>You should **ONLY** deploy OpenYurt on nodes that already have been joined in the Kubernetes cluster.</u>

##### 2.4.2.1 on the Worker Node

**<u>Firstly, on the worker node</u>**, use the following command:

```bash
./easy_openyurt yurt worker join -apiserver-advertise-address <apiserverAdvertiseAddress> -apiserver-token <apiserverToken>
# You can find these parameters in file `masterKey.yaml` previously introduced on the master node
# For Example:
./easy_openyurt yurt worker join -apiserver-advertise-address 192.168.18.2 -apiserver-token xxxxxxxxxx
```

To view the help and all available optional parameters, add `-h` to see more details:

```bash
./easy_openyurt yurt worker join -h
#### Output ####
# Usage of ./easy_openyurt yurt worker join:
#   -apiserver-advertise-address string
#         Kubernetes API server advertise address (**REQUIRED**)
#   -apiserver-port string
#         Kubernetes API server port (default "6443")
#   -apiserver-token string
#         Kubernetes API server token (**REQUIRED**)
#   -h    Show help
#   -help
#         Show help
```

##### 2.4.2.2 on the Master Node

**<u>Then, on the master node,</u>** use the following command:

```bash
./easy_openyurt yurt master expand -worker-node-name <nodeName> [-worker-as-edge]
# If you want to join the worker node as edge, specify the `-worker-as-edge` option
# <nodeName> is the name of the worker node that you want to join to the OpenYurt cluster
# For example:
./easy_openyurt yurt master expand -worker-node-name myEdgeNode0 -worker-as-edge
./easy_openyurt yurt master expand -worker-node-name myCloudNode0
```

To view the help and all available optional parameters, add `-h` to see more details:

```bash
./easy_openyurt yurt master expand -h
#### Output ####
# Usage of ./easy_openyurt yurt master expand:
#   -h    Show help
#   -help
#         Show help
#   -worker-as-edge
#         Treat worker as edge node (default true)
#   -worker-node-name string
#         Worker node name(**REQUIRED**)
```

### 2.5 Deploy Knative(vHive stock-only mode compatible)

 > This is an **optional** step. If you don't want to use the Knative/vHive, feel free to skip this section.

**<u>On the master node,</u>** use the following command to deploy Knative(vHive stock-only mode compatible) on the cluster:

```bash
./easy_openyurt knative master init
```

To view the help and all available optional parameters, add `-h` to see more details:

```bash
./easy_openyurt knative master init -h
#### Output ####
# Usage of ./easy_openyurt-amd64-linux-0.2.3 yurt master init:
  # -h    Show help
  # -help
        # Show help
  # -istio-version string
        # Istio version (default "1.16.3")
  # -knative-version string
        # Knative version (default "1.9.2")
  # -metalLB-version string
        # MetalLB version (default "0.13.9")
  # -vhive-mode
        # vHive mode (default true)
```

## 3. Create NodePool and deploy apps
Here we use a docker image named ```lrq619/srcnn``` as our example.

Below instructions should all be executed on master node.

### 3.1 Create NodePool
Create file called cloud.yaml
```yaml
apiVersion: apps.openyurt.io/v1alpha1
kind: NodePool
metadata:
  name: beijing # can change to your own name
spec:
  type: Cloud
```
Create file called edge.yaml
```yaml
apiVersion: apps.openyurt.io/v1alpha1
kind: NodePool
metadata:
  name: hangzhou # can change to your own name
spec:
  type: Edge
  annotations:
    apps.openyurt.io/example: test-hangzhou
  labels:
    apps.openyurt.io/example: test-hangzhou
  taints:
  - key: apps.openyurt.io/example
    value: test-hangzhou
    effect: NoSchedule
```
run
```bash
kubectl apply -f cloud.yaml
kubectl apply -f edge.yaml
kubectl get np
```
the output should be similar to 
```bash
NAME       TYPE    READYNODES   NOTREADYNODES   AGE
beijing    Cloud   1            0               67m
hangzhou   Edge    1            0               66m
```

### 3.2 Create YurtAppSet
Create a file yurtset.yaml
```yaml
# Used to create YurtAppSet
apiVersion: apps.openyurt.io/v1alpha1
kind: YurtAppSet
metadata:
  labels:
    controller-tools.k8s.io: "1.0"
  name: yas-test
spec:
  selector:
    matchLabels:
      app: yas-test
  workloadTemplate:
    deploymentTemplate:
      metadata:
        labels:
          app: yas-test
      spec:
        template:
          metadata:
            labels:
              app: yas-test
          spec:
            containers: # can be changed to your own images
              - name: srcnn
                image: lrq619/srcnn
                ports:
                - containerPort: 8000 # the port docker exposes
  topology:
    pools:
    - name: beijing # cloud nodepool name
      nodeSelectorTerm:
        matchExpressions:
        - key: apps.openyurt.io/nodepool
          operator: In
          values:
          - beijing
      replicas: 1
    - name: hangzhou # edge nodepool name
      nodeSelectorTerm:
        matchExpressions:
        - key: apps.openyurt.io/nodepool
          operator: In
          values:
          - hangzhou
      replicas: 1
      tolerations:
      - effect: NoSchedule
        key: apps.openyurt.io/example
        operator: Exists
  revisionHistoryLimit: 5
```
Then run
```bash
kubectl apply -f yurtset.yaml
```
The deployments is automatically created.
You can check them by
```bash
kubectl get deploy
```
It should output something like
```bash
NAME                      READY   UP-TO-DATE   AVAILABLE   AGE
yas-test-beijing-6bv5g    1/1     1            1           59m
yas-test-hangzhou-z22r4   1/1     1            1           59m
```
### 3.3 Expose deployments to external ip(Optional)
If the master node is running on a node with public ip address you can choose the expose the deployments to that address by:
```bash
kubectl expose deployment <deploy-name>  --type=LoadBalancer --target-port <container-exposed-ip> --external-ip <ip>
```
For example:
```bash
kubectl expose deployment yas-test-beijing-6bv5g  --type=LoadBalancer --target-port 8000 --external-ip 128.110.217.71
```
Then you can use
```bash
kubectl get services
```
to check the services' public ip addresses and ports to access them.
To delete a service, use 
```
kubectl delete svc <service-name>
```