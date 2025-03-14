#!/bin/bash
set -e

declare -A TEST_IMAGES
TEST_IMAGES["busybox"]="quay.io/chengyuzhu6/busybox:latest"
TEST_IMAGES["ubuntu"]="quay.io/chengyuzhu6/ubuntu:24.04"

declare -A TEST_COMMANDS
TEST_COMMANDS["busybox"]="sleep 120"
TEST_COMMANDS["ubuntu"]="sleep 120" 

export KUBECONFIG=/etc/kubernetes/admin.conf

echo "Running functional tests"

wait_for_pod() {
    local POD_NAME=$1
    local TIMEOUT=$2
    
    echo "Waiting for pod $POD_NAME to be ready (timeout: ${TIMEOUT}s)..."
    
    if ! kubectl wait --timeout="${TIMEOUT}s" --for=condition=ready "pods/$POD_NAME"; then
        echo "ERROR: Pod $POD_NAME failed to become ready within ${TIMEOUT}s"
        echo "Pod details:"
        kubectl describe pod "$POD_NAME"
        echo "Pod logs:"
        kubectl logs "$POD_NAME" || echo "Could not get logs for $POD_NAME"
        # echo "containerd logs:"
        # journalctl -t containerd
        echo "containerd-guest-pull-grpc logs:"
        cat /tmp/containerd-guest-pull-grpc.log
        return 1
    fi
    echo "Pod $POD_NAME is ready"
    kubectl describe pod "$POD_NAME"
    return 0
}

verify_image_integrity() {
    local IMAGE_NAME=$1
    
    echo "Verifying integrity of image: $IMAGE_NAME"
    
    echo "Checking image completeness..."
    local CHECK_OUTPUT=$(ctr -n k8s.io images check |grep "$IMAGE_NAME" 2>&1)
    ctr -n k8s.io images check |grep "$IMAGE_NAME"
    if echo "$CHECK_OUTPUT" | grep -q "incomplete"; then
        echo "Guest pull success: Image $IMAGE_NAME is not pulled in the host!"
        return 0
    else
        echo "Guest pull failure: Image $IMAGE_NAME is pulled in the host!"
        return 1
    fi
}

test_runtime_with_image() {
    local RUNTIME=$1
    local IMAGE_NAME=$2
    local IMAGE_URL=${TEST_IMAGES[$IMAGE_NAME]}
    local COMMAND=${TEST_COMMANDS[$IMAGE_NAME]}
    
    local POD_NAME="functional-test-${RUNTIME}-${IMAGE_NAME}"
    local YAML_FILE="/tmp/${POD_NAME}.yaml"
    
    echo "Testing runtime '$RUNTIME' with image '$IMAGE_NAME' ($IMAGE_URL)"
    
    cat > "$YAML_FILE" <<EOF
apiVersion: v1
kind: Pod
metadata:
  name: $POD_NAME
  annotations:
    io.containerd.cri.runtime-handler: ${RUNTIME}
spec:
  runtimeClassName: ${RUNTIME}
  containers:
  - name: guest-pull-container
    image: ${IMAGE_URL}
    command: ["/bin/sh", "-c", "${COMMAND}"]
EOF
    
    echo "Deploying $POD_NAME..."
    cat "$YAML_FILE"
    kubectl apply -f "$YAML_FILE"
    
    if ! wait_for_pod "$POD_NAME" 120; then
        echo "Test failed for runtime '$RUNTIME' with image '$IMAGE_NAME'"
        rm -f "$YAML_FILE"
        return 1
    fi

    if ! verify_image_integrity "${IMAGE_NAME}"; then
        echo "Image check failed for runtime '$RUNTIME' with image '$IMAGE_NAME'"
        kubectl delete -f "$YAML_FILE"
        rm -f "$YAML_FILE"
        return 1
    fi
    
    echo "Test passed for runtime '$RUNTIME' with image '$IMAGE_NAME'"
    kubectl delete -f "$YAML_FILE"
    rm -f "$YAML_FILE"
    return 0
}

run_all_tests() {    
    for IMAGE_NAME in "${!TEST_IMAGES[@]}"; do
        if ! test_runtime_with_image "kata-qemu-coco-dev" "$IMAGE_NAME"; then
            return 1
        fi
    done
    
    return 0
}

if run_all_tests; then
    echo "All functional tests passed successfully"
else
    echo "Some tests failed"
    exit 1
fi

echo "Cleaning up test resources..."
