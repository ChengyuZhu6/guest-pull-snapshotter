#!/bin/bash
set -e

# Source common test utilities
source tests/utils/test_utils.sh

# Define test images
declare -A TEST_IMAGES
TEST_IMAGES["busybox"]="quay.io/chengyuzhu6/busybox:latest"
TEST_IMAGES["nginx"]="quay.io/chengyuzhu6/nginx:latest"

echo "Running functional tests for guest-pull"

test_guest_pull_with_image() {
    local IMAGE_NAME=$1
    local IMAGE_URL=${TEST_IMAGES[$IMAGE_NAME]}
    local RUNTIME="kata-qemu-coco-dev"
    
    local POD_NAME="functional-test-${RUNTIME}-${IMAGE_NAME}"
    
    echo "Testing guest-pull with image '$IMAGE_NAME' ($IMAGE_URL)"
    
    # Deploy the pod
    deploy_test_pod "$RUNTIME" "$IMAGE_URL" "$POD_NAME"
    
    # Wait for pod to be ready
    if ! wait_for_pod "$POD_NAME" 120; then
        echo "Test failed: Pod did not become ready"
        delete_pod "$POD_NAME"
        return 1
    fi

    # Verify image integrity
    if ! verify_image_integrity "$IMAGE_NAME"; then
        echo "Test failed: Image integrity check failed"
        delete_pod "$POD_NAME"
        return 1
    fi
    
    echo "Test passed for image '$IMAGE_NAME'"
    delete_pod "$POD_NAME"
    return 0
}

run_all_tests() {
    local FAILED=0
    
    for IMAGE_NAME in "${!TEST_IMAGES[@]}"; do
        if ! test_guest_pull_with_image "$IMAGE_NAME"; then
            FAILED=1
        fi
    done
    
    return $FAILED
}

# Run all tests and report results
if run_all_tests; then
    echo "✅ All functional tests passed successfully"
else
    echo "❌ Some tests failed"
    exit 1
fi

echo "Cleaning up test resources..."
