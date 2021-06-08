#!/bin/bash

set -e

BASEDIR=$(dirname "${0}")

echo "Namespace: applying..."
kubectl apply --filename "${BASEDIR}/0-namespace.yaml"
echo "Namespace: ready!"
echo

# Save the namespace for all subsequent kubectl commands in this context:
# https://kubernetes.io/docs/concepts/overview/working-with-objects/namespaces/#setting-the-namespace-for-a-request
kubectl config set-context --current --namespace=chained-functions-eventing
echo

echo "Broker: applying..."
kubectl apply --filename "${BASEDIR}/1-broker.yaml"
echo "Broker: waiting..."
kubectl wait --for=condition=Ready --timeout=120s broker/default || { kubectl describe broker/default; exit 1; }
echo "Broker: ready!"
echo

echo "Ksvc: applying..."
kubectl apply --filename "${BASEDIR}/2-ksvc.yaml"
echo "Ksvc: waiting consumer..."
kubectl wait --for=condition=Ready --timeout=120s ksvc/consumer || { kubectl describe ksvc/consumer; exit 1; }
echo "Ksvc: waiting producer..."
kubectl wait --for=condition=Ready --timeout=120s ksvc/producer || { kubectl describe ksvc/producer; exit 1; }
echo "Ksvc: both ready!"
echo

echo "SinkBinding: applying..."
kubectl apply --filename "${BASEDIR}/3-sinkbinding.yaml"
echo "SinkBinding: waiting..."
kubectl wait --for=condition=Ready --timeout=120s sinkbinding/producer-binding || { kubectl describe sinkbinding/producer-binding; exit 1; }
echo "SinkBinding: ready!"
echo

echo "Trigger: applying..."
kubectl apply --filename "${BASEDIR}/4-trigger.yaml"
echo "Trigger: waiting..."
kubectl wait --for=condition=Ready --timeout=120s trigger/default-consumer-trigger || { kubectl describe trigger/default-consumer-trigger; exit 1; }
echo "Trigger: ready!"
