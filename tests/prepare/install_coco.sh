#!/bin/bash

set -e

COCO_VERSION=${COCO_VERSION:-"v0.12.0"}

echo "Installing CoCo ${COCO_VERSION}"
export KUBECONFIG=/etc/kubernetes/admin.conf

kubectl apply -k "github.com/confidential-containers/operator/config/release?ref=${COCO_VERSION}"
kubectl apply -k "github.com/confidential-containers/operator/config/samples/ccruntime/default?ref=${COCO_VERSION}"

echo "Waiting for cc-operator-daemon-install pods to be running..."
kubectl wait --for=condition=Ready pods -l app=cc-operator-daemon-install --timeout=300s -n confidential-containers-system

echo "CoCo installation completed successfully"



