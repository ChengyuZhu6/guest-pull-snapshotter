#!/bin/bash
set -e

# Source common test utilities
source tests/utils/test_utils.sh

# Define test images
declare -A TEST_IMAGES
TEST_IMAGES["busybox"]="quay.io/chengyuzhu6/busybox:latest"
TEST_IMAGES["ubuntu"]="quay.io/chengyuzhu6/ubuntu:24.04"
TEST_IMAGES["nginx"]="quay.io/chengyuzhu6/nginx:latest"

echo "Running compatibility tests for runc and kata-qemu-coco-dev"

test_runtime_with_image() {
    local RUNTIME=$1
    local IMAGE_NAME=$2
    local IMAGE_URL=${TEST_IMAGES[$IMAGE_NAME]}
    
    local POD_NAME="compat-test-${RUNTIME}-${IMAGE_NAME}"
    
    echo "Testing runtime '$RUNTIME' with image '$IMAGE_NAME' ($IMAGE_URL)"
    
    # Deploy the pod
    deploy_test_pod "$RUNTIME" "$IMAGE_URL" "$POD_NAME"
    
    # Wait for pod to be ready
    if ! wait_for_pod "$POD_NAME" 120; then
        echo "Test failed for runtime '$RUNTIME' with image '$IMAGE_NAME'"
        delete_pod "$POD_NAME"
        return 1
    fi
    
    echo "Test passed for runtime '$RUNTIME' with image '$IMAGE_NAME'"
    delete_pod "$POD_NAME"
    return 0
}

run_all_tests() {
    
    echo "Testing runc runtime..."
    for IMAGE_NAME in "${!TEST_IMAGES[@]}"; do
        if ! test_runtime_with_image "runc" "$IMAGE_NAME"; then
            return 1
        fi
    done

    echo "Testing kata-qemu-coco-dev runtime..."
    for IMAGE_NAME in "${!TEST_IMAGES[@]}"; do
        if ! test_runtime_with_image "kata-qemu-coco-dev" "$IMAGE_NAME"; then
            return 1
        fi
    done
    
    return 0
}

# Run all tests and report results
if run_all_tests; then
    echo "✅ All compatibility tests passed successfully"
else
    echo "❌ Some tests failed"
    exit 1
fi

echo "Cleaning up test resources..."
