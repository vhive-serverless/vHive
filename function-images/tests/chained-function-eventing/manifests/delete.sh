#!/bin/bash

BASEDIR=$(dirname "${0}")

# Save the namespace for all subsequent kubectl commands in this context:
# https://kubernetes.io/docs/concepts/overview/working-with-objects/namespaces/#setting-the-namespace-for-a-request
kubectl config set-context --current --namespace=chained-functions-eventing

kubectl delete --filename "${BASEDIR}/4-trigger.yaml"
kubectl delete --filename "${BASEDIR}/3-sinkbinding.yaml"
kubectl delete --filename "${BASEDIR}/2-ksvc.yaml"
kubectl delete --filename "${BASEDIR}/1-broker.yaml"
kubectl delete --filename "${BASEDIR}/0-namespace.yaml"
