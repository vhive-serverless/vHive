#!/bin/bash

helm uninstall minio-tenant --namespace minio
helm uninstall minio-operator --namespace minio
kubectl delete namespace minio
kubectl delete pvc --all --namespace minio
kubectl delete -f configs/storage/minio/pv.yaml
tmux kill-session -t portforward-minio
