#!/bin/bash
set -e

# Source common test utilities
source tests/utils/test_utils.sh

echo "Running functional tests for guest-pull"

run_all_tests() {
    local FAILED=0
    local RUNTIME="kata-qemu-coco-dev"
    
    for IMAGE_NAME in "${!TEST_IMAGES[@]}"; do
        if ! run_pod_test "$RUNTIME" "$IMAGE_NAME" "functional" true; then
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
