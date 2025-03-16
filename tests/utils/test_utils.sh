#!/bin/bash

# Common test utilities for all test scripts

export KUBECONFIG=/etc/kubernetes/admin.conf

# Common test images used across all tests
declare -A TEST_IMAGES
TEST_IMAGES["busybox"]="quay.io/chengyuzhu6/busybox:latest"
TEST_IMAGES["ubuntu"]="quay.io/chengyuzhu6/ubuntu:24.04"
TEST_IMAGES["nginx"]="quay.io/chengyuzhu6/nginx:latest"

print_header() {
    local TITLE=$1
    echo ""
    echo "================================================================"
    echo "  ${TITLE}"
    echo "================================================================"
    echo ""
}

print_subheader() {
    local TITLE=$1
    echo ""
    echo "----------------------------------------------------------------"
    echo "  ${TITLE}"
    echo "----------------------------------------------------------------"
    echo ""
}

# Wait for pod to be ready
wait_for_pod() {
    local POD_NAME=$1
    local TIMEOUT=${2:-120}
    
    echo "Waiting for pod $POD_NAME to be ready (timeout: ${TIMEOUT}s)..."
    
    if ! kubectl wait --timeout="${TIMEOUT}s" --for=condition=ready "pods/$POD_NAME"; then
        echo "ERROR: Pod $POD_NAME failed to become ready within ${TIMEOUT}s"
        kubectl describe pod "$POD_NAME"
        kubectl logs "$POD_NAME" 2>/dev/null || true
        return 1
    fi
    
    echo "Pod $POD_NAME is ready"
    return 0
}

# Wait for pod deletion
wait_for_pod_deletion() {
    local POD_NAME=$1
    local TIMEOUT=${2:-60}
    local INTERVAL=2
    local ELAPSED=0
    
    echo "Waiting for pod '$POD_NAME' to be deleted (timeout: ${TIMEOUT}s)..."
    
    while [ $ELAPSED -lt $TIMEOUT ]; do
        if ! kubectl get pod "$POD_NAME" &>/dev/null; then
            echo "Pod '$POD_NAME' has been successfully deleted"
            return 0
        fi
        
        sleep $INTERVAL
        ELAPSED=$((ELAPSED + INTERVAL))
        echo "Still waiting for pod deletion... ($ELAPSED/$TIMEOUT seconds)"
    done
    
    echo "Timeout reached. Pod '$POD_NAME' was not deleted within ${TIMEOUT} seconds"
    return 1
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
    imagePullPolicy: Always
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
    
    echo "Deleting pod $POD_NAME..."
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

    echo "Check image $IMAGE_NAME to ensure the image data content is not available locally"
    ctr -n k8s.io images check | grep "$IMAGE_NAME"
    if ctr -n k8s.io images check | grep "$IMAGE_NAME" | grep -q "incomplete"; then
        echo "Guest pull success: Image $IMAGE_NAME is not pulled in the host!"
        return 0
    else
        echo "Guest pull failure: Image $IMAGE_NAME is pulled in the host!"
        return 1
    fi
}

# Run a complete test cycle for a pod
run_pod_test() {
    local RUNTIME=$1
    local IMAGE_NAME=$2
    local TEST_TYPE=$3
    local VERIFY_INTEGRITY=${4:-false}
    
    local IMAGE_URL=${TEST_IMAGES[$IMAGE_NAME]}
    local POD_NAME="${TEST_TYPE}-test-${RUNTIME}-${IMAGE_NAME}"
    
    print_subheader "Testing $RUNTIME with image $IMAGE_NAME"
    echo "Image URL: $IMAGE_URL"
    
    deploy_test_pod "$RUNTIME" "$IMAGE_URL" "$POD_NAME"
    
    if ! wait_for_pod "$POD_NAME" 120; then
        echo "❌ Test failed for runtime '$RUNTIME' with image '$IMAGE_NAME'"
        delete_pod "$POD_NAME"
        return 1
    fi
    
    # Verify image integrity if requested (for guest-pull tests)
    if [ "$VERIFY_INTEGRITY" = true ] && ! verify_image_integrity "$IMAGE_NAME"; then
        echo "❌ Test failed: Image integrity check failed for '$IMAGE_NAME'"
        delete_pod "$POD_NAME"
        return 1
    fi
    
    echo "✅ Test passed for runtime '$RUNTIME' with image '$IMAGE_NAME'"
    delete_pod "$POD_NAME"
    
    if ! wait_for_pod_deletion "$POD_NAME" 60; then
        echo "⚠️ Warning: Pod deletion timed out"
        return 1
    fi

    return 0
}

# Find and kill the guest-pull snapshotter process with specified signal
kill_snapshotter_process() {
    local SIGNAL=$1
    
    echo "Finding and killing containerd-guest-pull-grpc process with signal ${SIGNAL}..."
    local GRPC_PID=$(pgrep -f "containerd-guest-pull-grpc" || echo "")
    
    if [ -z "$GRPC_PID" ]; then
        echo "Failed to find containerd-guest-pull-grpc process"
        return 1
    fi
    
    echo "Found containerd-guest-pull-grpc process with PID: $GRPC_PID"
    kill -${SIGNAL} $GRPC_PID
    echo "Sent signal ${SIGNAL} to process ${GRPC_PID}"
    
    echo "Restarting guest-pull snapshotter service..."
    sudo systemctl restart guest-pull-snapshotter
    sudo systemctl status guest-pull-snapshotter
    
    return 0
} 