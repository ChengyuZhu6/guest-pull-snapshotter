#!/bin/bash
#
# Compatibility Test Suite for Guest Pull Snapshotter
# ==================================================
#
# Purpose:
#   This test verifies that both runc and kata-qemu-coco-dev runtimes can 
#   successfully run containers with various images. It ensures that the 
#   guest-pull snapshotter is compatible with different container runtimes
#   and doesn't break standard container functionality.
#
# Test Strategy:
#   1. For each runtime (runc, kata-qemu-coco-dev):
#      - Deploy pods with different images (busybox, ubuntu, nginx)
#      - Verify pods become ready
#      - Clean up resources
#
# Expected Results:
#   - All pods should start successfully with both runtimes
#   - No errors should occur during pod lifecycle
#

set -e

source tests/utils/test_utils.sh

print_header "COMPATIBILITY TESTS"
echo "Testing compatibility between runc and kata-qemu-coco-dev runtimes"

run_all_tests() {
    local RUNTIMES=("runc" "kata-qemu-coco-dev")

    for RUNTIME in "${RUNTIMES[@]}"; do
        print_subheader "Testing $RUNTIME runtime"
        for IMAGE_NAME in "${!TEST_IMAGES[@]}"; do
            if ! run_pod_test "$RUNTIME" "$IMAGE_NAME" "compat"; then
                return 1
            fi
        done
    done
    
    return 0
}

if run_all_tests; then
    print_header "COMPATIBILITY TESTS: PASSED ✅"
else
    print_header "COMPATIBILITY TESTS: FAILED ❌"
    exit 1
fi
