#!/bin/bash

set -e

BASEDIR=$(dirname "${0}")

echo "Namespace: applying..."
kubectl apply --filename "${BASEDIR}/0-namespace.yaml"
echo "Namespace: ready!"
echo

echo "Broker: applying..."
kubectl apply --filename "${BASEDIR}/1-broker.yaml"
echo "Broker: waiting..."
kubectl wait -n=chained-functions-eventing --for=condition=Ready --timeout=120s broker/default || { kubectl describe broker/default; exit 1; }
echo "Broker: ready!"
echo

echo "Deployment: applying..."
kubectl apply --filename "${BASEDIR}/2-deployment.yaml"
echo "Deployment: waiting timeseriesdb..."
kubectl wait -n=chained-functions-eventing --for=condition=Available --timeout=120s deployment/timeseriesdb || { kubectl describe deployment/timeseriesdb; exit 1; }
echo "Deployment: ready!"
echo

echo "Svc: applying..."
kubectl apply --filename "${BASEDIR}/3-svc.yaml"
echo "Svc: ready!"
echo

echo "Ksvc: applying..."
kubectl apply --filename "${BASEDIR}/4-ksvc.yaml"
echo "Ksvc: waiting consumer..."
kubectl wait -n=chained-functions-eventing --for=condition=Ready --timeout=120s ksvc/consumer || { kubectl describe ksvc/consumer; exit 1; }
echo "Ksvc: waiting producer..."
kubectl wait -n=chained-functions-eventing --for=condition=Ready --timeout=120s ksvc/producer || { kubectl describe ksvc/producer; exit 1; }
echo "Ksvc: both ready!"
echo

echo "SinkBinding: applying..."
kubectl apply --filename "${BASEDIR}/5-sinkbinding.yaml"
echo "SinkBinding: waiting..."
kubectl wait -n=chained-functions-eventing --for=condition=Ready --timeout=120s sinkbinding/producer-binding || { kubectl describe sinkbinding/producer-binding; exit 1; }
echo "SinkBinding: ready!"
echo

echo "Trigger: applying..."
kubectl apply --filename "${BASEDIR}/6-trigger.yaml"
echo "Trigger: waiting default-timeseriesdb-trigger..."
kubectl wait -n=chained-functions-eventing --for=condition=Ready --timeout=120s trigger/default-timeseriesdb-trigger || { kubectl describe trigger/default-timeseriesdb-trigger; exit 1; }
echo "Trigger: waiting default-consumer-trigger..."
kubectl wait -n=chained-functions-eventing --for=condition=Ready --timeout=120s trigger/default-consumer-trigger || { kubectl describe trigger/default-consumer-trigger; exit 1; }
echo "Trigger: ready!"
