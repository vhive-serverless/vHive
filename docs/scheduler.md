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

```bash
kubectl apply -f k8s/nodesnapshotcache-crd.yaml
```

