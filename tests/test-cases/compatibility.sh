#!/bin/bash
set -e

# Source common test utilities
source tests/utils/test_utils.sh

echo "Running compatibility tests for runc and kata-qemu-coco-dev"

run_all_tests() {
    local FAILED=0
    local RUNTIMES=("runc" "kata-qemu-coco-dev")

    for IMAGE_NAME in "${!TEST_IMAGES[@]}"; do
        echo "Testing $RUNTIME runtime..."
            for RUNTIME in "${RUNTIMES[@]}"; do
            if ! run_pod_test "$RUNTIME" "$IMAGE_NAME" "compat"; then
                FAILED=1
            fi
        done
    done
    
    return $FAILED
}

# Run all tests and report results
if run_all_tests; then
    echo "✅ All compatibility tests passed successfully"
else
    echo "❌ Some tests failed"
    exit 1
fi

echo "Cleaning up test resources..."
