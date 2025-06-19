# Snapshot-aware Scheduler

...

1. 

```bash
kubectl apply -f k8s/nodesnapshotcache-crd.yaml
```
go install sigs.k8s.io/controller-tools/cmd/controller-gen@v0.14.0
export PATH=$PATH:$HOME/go/bin

controller-gen object paths="./k8s"
