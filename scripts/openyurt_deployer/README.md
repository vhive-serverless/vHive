# Quick set-up OpenYurt

## 1. Introduction

This program extends [EasyOpenyurt](https://github.com/flyinghorse0510/easy_openyurt) to automate the set up process of an OpenYurt cluster. 

It support seting up a Kubernetes cluster using kubeadm and then deploy OpenYurt and Knative on it. It is compatible with vHive stock-only mode.

## 2. Brief overview

**Pre-Requsites of nodes:**
1. The scripts has been tested on [cloud-lab](https://www.cloudlab.us/), suggested profile is: [openyurt-demo](https://www.cloudlab.us/p/ntu-cloud/openyurt-demo), with one master node, one cloud worker node and one edge worker node
2. Ensure that SSH authentication is possible from local device to all nodes.
 

**Components:**

|      Files      | Purpose  |
| :----------: | :---: |
| main.go | script entry point |
| conf.json | json files that stores cluster's configuration |
| node | executing commands on remote nodes through ssh |
| configs | node runtime configurations |

**Description**

1. Prepare system environment for all nodes, installing kubeadm, kubectl, dependencies, etc.
2. On master node, init the cluster using `kubeadm init` and in each worker node, join the initialized cluster.
3. On top of the created cluster, init openyurt cluster both on master nodes and worker nodes, then expand to all worker nodes from master nodes.
4. (Optional) Deploy Knative (vHive stock-only mode compatible)

## 3. Usage
```bash
./openyurt_deployer deploy # deploy openyurt on the cluster 
```
```bash
./openyurt_deployer clean # clean the openyurt cluster and restore it to initial state 
```

### 3.1 Preparations 
1. Prepare a cluster with at least two nodes.
2. Change the contents in `conf.json` to following format:
```plaintext
{
  "master": "user@master",
  "workers": {
    "cloud": [
      "user@cloud-0"
    ],
    "edge": [
      "user@edge-0"
    ]
  }
}
```

### 3.2 Run Script

```bash
go build .
./openyurt_deployer deploy
```
If it gives out error like: 
```
FATA[0001] Failed to connect to: username@host
```
Please execute:
```
eval `ssh-agent -s` && ssh-add ~/.ssh/<your private key>
```
For example:
```
eval `ssh-agent -s` && ssh-add ~/.ssh/id_rsa
```
And try again


## 4. Demo: Create NodePool
**Referenced from [OpenYurt](https://openyurt.io/docs/user-manuals/workload/node-pool-management)*


Below instructions should all be executed on master node.

### 4.1 Create NodePool
1. Create file called cloud.yaml
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
2. Label the nodes with corresponding label
run
```bash
kubectl label node {Cloud_Node_Name} apps.openyurt.io/desired-nodepool={Cloud_Nodepool_Name}
```
```bash
kubectl label node {Edge_Node_Name} apps.openyurt.io/desired-nodepool={Cloud_Nodepool_Name}
```
3. Create Nodepool and see the outputs
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
