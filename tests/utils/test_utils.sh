#!/bin/bash

# Common test utilities for all test scripts

# Set up environment
export KUBECONFIG=/etc/kubernetes/admin.conf

# Wait for pod to be ready
wait_for_pod() {
    local POD_NAME=$1
    local TIMEOUT=${2:-120}
    
    echo "Waiting for pod $POD_NAME to be ready (timeout: ${TIMEOUT}s)..."
    
    if ! kubectl wait --timeout="${TIMEOUT}s" --for=condition=ready "pods/$POD_NAME"; then
        echo "ERROR: Pod $POD_NAME failed to become ready within ${TIMEOUT}s"
        echo "Pod details:"
        kubectl describe pod "$POD_NAME"
        echo "Pod logs:"
        kubectl logs "$POD_NAME" || echo "Could not get logs for $POD_NAME"
        return 1
    fi
    
    echo "Pod $POD_NAME is ready"
    return 0
}

# Create and deploy a pod with specified runtime and image
deploy_test_pod() {
    local RUNTIME=$1
    local IMAGE=$2
    local POD_NAME=$3
    local YAML_FILE="/tmp/${POD_NAME}.yaml"
    
    echo "Creating pod $POD_NAME with runtime '$RUNTIME' and image '$IMAGE'"
    
    if [ "$RUNTIME" == "runc" ]; then
        cat > "$YAML_FILE" <<EOF
apiVersion: v1
kind: Pod
metadata:
  name: ${POD_NAME}
spec:
  containers:
  - name: test-container
    image: ${IMAGE}
    command:
      - sleep
      - "120"
EOF
    else
        cat > "$YAML_FILE" <<EOF
apiVersion: v1
kind: Pod
metadata:
  name: ${POD_NAME}
  annotations:
    io.containerd.cri.runtime-handler: ${RUNTIME}
spec:
  runtimeClassName: ${RUNTIME}
  containers:
  - name: test-container
    image: ${IMAGE}
    command:
      - sleep
      - "120"
    imagePullPolicy: Always
EOF
    fi
    
    cat "$YAML_FILE"

    kubectl apply -f "$YAML_FILE"
    echo "Pod $POD_NAME created"
    
    return 0
}

# Clean up a pod
delete_pod() {
    local POD_NAME=$1
    local YAML_FILE="/tmp/${POD_NAME}.yaml"
    
    if [ -f "$YAML_FILE" ]; then
        kubectl delete -f "$YAML_FILE" --ignore-not-found
        rm -f "$YAML_FILE"
    else
        kubectl delete pod "$POD_NAME" --ignore-not-found
    fi
}

# Verify image integrity for guest-pull
verify_image_integrity() {
    local IMAGE_NAME=$1
    
    echo "Verifying integrity of image: $IMAGE_NAME"
    
    echo "Checking image completeness..."
    if ctr -n k8s.io images check | grep "$IMAGE_NAME" | grep -q "incomplete"; then
        echo "Guest pull success: Image $IMAGE_NAME is not pulled in the host!"
        return 0
    else
        echo "Guest pull failure: Image $IMAGE_NAME is pulled in the host!"
        return 1
    fi
} 