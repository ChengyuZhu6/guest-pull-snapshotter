#!/bin/bash
#
# Functional Test Suite for Guest Pull Snapshotter
# ===============================================
#
# Purpose:
#   This test verifies the core functionality of the guest-pull snapshotter
#   with kata-qemu-coco-dev runtime. It ensures that images are properly
#   pulled inside the guest VM rather than on the host.
#
# Test Strategy:
#   1. Deploy pods with different images using kata-qemu-coco-dev runtime
#   2. Verify pods become ready
#   3. Verify image integrity by checking that image data is not available on host
#   4. Clean up resources
#
# Expected Results:
#   - All pods should start successfully
#   - Image data should not be present on the host (incomplete flag)
#   - No errors should occur during pod lifecycle
#

set -e

source tests/utils/test_utils.sh

print_header "FUNCTIONAL TESTS"
echo "Testing guest-pull functionality with kata-qemu-coco-dev runtime"

run_all_tests() {
    local RUNTIME="kata-qemu-coco-dev"
    
    for IMAGE_NAME in "${!TEST_IMAGES[@]}"; do
        if ! run_pod_test "$RUNTIME" "$IMAGE_NAME" "functional" true; then
            return 1
        fi
    done
    
    return 0
}

if run_all_tests; then
    print_header "FUNCTIONAL TESTS: PASSED ✅"
else
    print_header "FUNCTIONAL TESTS: FAILED ❌"
    exit 1
fi
