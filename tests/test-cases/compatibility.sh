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

run_compatibility_tests() {
    local RUNTIMES=("runc" "kata-qemu-coco-dev")
    echo "Testing compatibility between runc and kata-qemu-coco-dev runtimes"

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

run_test_suite "COMPATIBILITY TESTS" run_compatibility_tests || exit 1
