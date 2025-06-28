# Scheduler

The scheduler is responsible for scheduling the pods in the Kubernetes cluster.
In Knative, the scheduler is responsible for scheduling the Knative services, which are built on top of Kubernetes pods.

vHive can be configured using one of the following schedulers:

- **Default Kubernetes Scheduler**: The default scheduler that comes with Kubernetes.
- **Snapshot-Aware Scheduler**: A custom scheduler that is aware of the snapshot cache in each node and can schedule pods based on the available snapshots.

## Snapshot-Aware Scheduler

The snapshot-aware scheduler is a custom scheduler that is aware of the snapshot cache in each node and can schedule pods based on the available snapshots. It uses the [`NodeSnapshotCache`](../configs/k8s/nodesnapshotcache-crd.yaml) Custom Resource Definition (CRD) to store the snapshot cache in each node.

The scheduler uses a scheduler extender, which is a webhook that permits a remote HTTP backend to filter and/or prioritize the nodes that the kube-scheduler chooses for a pod. For this scheduler, we only use the **prioritization** feature of the scheduler extender, giving higher priority to nodes that have the required snapshot in their cache.

- [Scheduler extender configuration](../configs/k8s/scheduler-extender-deployment.yaml)
- [Scheduler deployment configuration](../configs/k8s/scheduler-deployment-with-extender.yaml)
- [NodeSnapshotCache CRD configuration](../configs/k8s/nodesnapshotcache-crd.yaml)
- [Scheduler extender code](../k8s/scheduler/extender/)

The deployment of the scheduler is automated using the `setup_tool` (check details in the [Quickstart guide](./quickstart_guide.md)).

However, currently, in order to snapshot managers of worker nodes to be able to create the `NodeSnapshotCache` CRD, you need to create a kubeconfig file in each worker node:

1. On the master node, run:

   ```bash
   sudo cat /etc/kubernetes/admin.conf
   ```

   Copy the output of the command above, which is the kubeconfig file content.

2. On the worker node, create the kubeconfig file:

   ```bash
   mkdir -p ~/.kube
   nano ~/.kube/config
   ```

   Paste the content of the kubeconfig file you copied from the master node into the `config` file.

3. Set proper permissions for the kubeconfig file:

   ```bash
   chmod 600 ~/.kube/config
   ```

4. Verify that the kubeconfig file is working by running:
   ```bash
   kubectl get nodes
   ```
   You should see the list of nodes in the cluster.
