#!/bin/bash

set -e

COCO_VERSION=${COCO_VERSION:-"v0.12.0"}

echo "Installing CoCo ${COCO_VERSION}"
export KUBECONFIG=/etc/kubernetes/admin.conf
kubectl apply -k "github.com/confidential-containers/operator/config/release?ref=${COCO_VERSION}"
kubectl apply -k "github.com/confidential-containers/operator/config/samples/ccruntime/default?ref=${COCO_VERSION}"

echo "Waiting for all pods to be Running and Ready..."
    
TIMEOUT=600
INTERVAL=10
ELAPSED=0

check_required_pods() {
    local POD_PREFIXES=("cc-operator-controller-manager" "cc-operator-daemon-install" "cc-operator-pre-install-daemon")
    
    for PREFIX in "${POD_PREFIXES[@]}"; do
        POD_COUNT=$(kubectl get pods -n confidential-containers-system | grep "^$PREFIX" | wc -l)
        if [ "$POD_COUNT" -eq 0 ]; then
            echo "$PREFIX pod not found yet"
            return 1
        fi
        
        NOT_RUNNING=$(kubectl get pods -n confidential-containers-system | grep "^$PREFIX" | grep -v "Running" | wc -l)
        if [ "$NOT_RUNNING" -gt 0 ]; then
            echo "$PREFIX pod is not in Running state yet"
            return 1
        fi
    done
    
    return 0
}
    
while [ $ELAPSED -lt $TIMEOUT ]; do
    if check_required_pods; then
        echo "All required pods are running"
        
        if kubectl wait --for=condition=Ready pods --all -n confidential-containers-system --timeout=5s >/dev/null 2>&1; then
            echo "All pods are Ready!"
            break
        else
            echo "Required pods are running but not all are ready yet"
        fi
    fi
        
    echo "Some pods are not ready yet, waiting... ($ELAPSED/$TIMEOUT seconds)"
    kubectl get pods -n confidential-containers-system
    sleep $INTERVAL
    ELAPSED=$((ELAPSED + INTERVAL))
done
    
if [ $ELAPSED -ge $TIMEOUT ]; then
    echo "Timeout waiting for pods to be ready"
    kubectl get pods -n confidential-containers-system
    exit 1
fi
sleep 10
kubectl get pods -n confidential-containers-system
kubectl get runtimeclass

echo "Check the containerd config file"
cat /etc/containerd/config.toml

echo "CoCo installation completed successfully"
