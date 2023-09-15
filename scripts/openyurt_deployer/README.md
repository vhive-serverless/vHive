# Quick set-up `OpenYurt`

## 1. Introduction

This program extends [`EasyOpenyurt`](https://github.com/flyinghorse0510/easy_openyurt) to automate the set up process of an `OpenYurt` cluster. 

It support setting up a Kubernetes cluster using kubeadm and then deploy `OpenYurt` and Knative on it. It is compatible with vHive stock-only mode.

## 2. Brief overview

**Prerequisite of nodes:**
1. The scripts has been tested on [cloud-lab](https://www.cloudlab.us/), suggested profile is: [`openyurt-demo`](https://www.cloudlab.us/p/ntu-cloud/openyurt-demo), with one master node, one cloud worker node and one edge worker node
2. Ensure that SSH authentication is possible from local device to all nodes.
 

**Components:**

|      Files      | Purpose  |
| :----------: | :---: |
| main.go | script entry point |
| `conf.json` | json files that stores cluster's configuration |
| node | executing commands on remote nodes through ssh |
| configs | node runtime configurations |

**Description**

1. Prepare system environment for all nodes, installing kubeadm, kubectl, dependencies, etc.
2. On master node, init the cluster using `kubeadm init` and in each worker node, join the initialized cluster.
3. On top of the created cluster, init `openyurt` cluster both on master nodes and worker nodes, then expand to all worker nodes from master nodes.
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


## 4. Demo: Create `NodePool` And Deploy service on it
**Referenced from [`OpenYurt`](https://openyurt.io/docs/user-manuals/workload/node-pool-management)*

The demo would deploy a helloworld function to cloud node pool or edge node pool

Deploy the demo:
```
./openyurt_deployer demo-c
```
or:
```
./openyurt_deployer demo-e
```
where `demo-c` would deploy the service to the cloud node pool and `demo-e` would deploy the service to the edge node pool.

The demo code will also show information about node pool after deployment.
The name for `demo-c` would be `helloworld-cloud`, while the name for `demo-e` would be `helloworld-edge`
It will also show the services' `URL` so you can try to invoke it on the master node.

You can check the nodepool information simply by:
```
./openyurt_deployer demo-print
```
Or delete the services deployed on nodepool by:
```
./openyurt_deployer demo-clear
```


### 4.1 Invoke the Services (Optional)
You can try to invoke the services created by `demo-c` or `demo-e` on master node.
First, ssh to master node, following commands should all be executed on master node.
```
ssh <master-user>@<master-ip>
git clone https://github.com/vhive-serverless/vSwarm.git
cd vSwarm/tools/test-client && go build .
./test-client --addr $URL:80 --name "Hello there"
```

Here `$URL` should be the `URL` returned in the previous part when deploying cloud and edge services, you can also get it from: `kubectl get ksvc`, but discarding the `http://` at the beginning.
 
After invoking, you can use `kubectl get pods -o wide` to check whether the pods have been auto-scaled.