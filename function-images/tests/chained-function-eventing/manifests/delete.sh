#!/bin/bash

BASEDIR=$(dirname "${0}")

kubectl delete --filename "${BASEDIR}/6-trigger.yaml"
kubectl delete --filename "${BASEDIR}/5-sinkbinding.yaml"
kubectl delete --filename "${BASEDIR}/4-ksvc.yaml"
kubectl delete --filename "${BASEDIR}/3-svc.yaml"
kubectl delete --filename "${BASEDIR}/2-deployment.yaml"
kubectl delete --filename "${BASEDIR}/1-broker.yaml"
kubectl delete --filename "${BASEDIR}/0-namespace.yaml"
