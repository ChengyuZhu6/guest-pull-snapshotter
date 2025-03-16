#!/bin/bash
#
# Stability Test Suite for Guest Pull Snapshotter
# ==============================================
#
# Purpose:
#   This test verifies the stability and resilience of the guest-pull snapshotter
#   service when subjected to different termination signals. It ensures that the
#   service can recover properly and continue functioning after being killed.
#
# Test Strategy:
#   For each signal type (SIGINT, SIGTERM, SIGKILL):
#   1. Deploy an initial pod with kata-qemu-coco-dev runtime
#   2. Kill the guest-pull snapshotter process with the specific signal
#   3. Verify the initial pod can still become ready
#   4. Deploy a second pod with kata-qemu-coco-dev to verify pod restart
#   5. Deploy a third pod with runc to verify system-wide stability
#   6. Clean up all resources
#
# Expected Results:
#   - Service should restart successfully after each signal
#   - Pods should become ready even after service disruption
#   - Both kata and runc runtimes should work after service restart
#

set -e

source tests/utils/test_utils.sh

print_header "STABILITY TESTS"
echo "Testing system stability with various signals to guest-pull snapshotter"

# Run a complete test cycle for a specific signal
test_kill_signal() {
    local SIGNAL=$1
    local SIGNAL_NAME=$2
    local RUNTIME="kata-qemu-coco-dev"
    local IMAGE_NAME="nginx"
    local POD_NAME="stability-test-signal-${SIGNAL}"
    local RESTART_POD_NAME="${POD_NAME}-restart"
    local IMAGE_URL=${TEST_IMAGES[$IMAGE_NAME]}
    
    print_subheader "Testing stability with signal ${SIGNAL} (${SIGNAL_NAME})"
    
    # Step 1: Deploy initial pod
    echo "1. Deploying initial test pod..."
    deploy_test_pod "$RUNTIME" "$IMAGE_URL" "$POD_NAME"
    sleep 3
    
    # Step 2: Kill the process
    if ! kill_snapshotter_process "$SIGNAL"; then
        delete_pod "$POD_NAME"
        return 1
    fi
    
    # Step 3: Check if pod can still become ready
    echo "3. Checking if pod can still become ready after kill..."
    if ! wait_for_pod "$POD_NAME" 120; then
        echo "❌ Test failed: Pod did not become ready after killing process with signal ${SIGNAL}"
        delete_pod "$POD_NAME"
        return 1
    fi
    
    # Step 4: Clean up first pod
    echo "4. Pod became ready successfully after kill. Deleting pod..."
    delete_pod "$POD_NAME"
    if ! wait_for_pod_deletion "$POD_NAME" 60; then
        echo "⚠️ Warning: Pod deletion timed out"
        return 1
    fi
    
    # Step 5: Test recovery with a second pod using kata runtime
    echo "5. Deploying second test pod with kata runtime to verify system restart..."
    deploy_test_pod "$RUNTIME" "$IMAGE_URL" "$RESTART_POD_NAME"
    if ! wait_for_pod "$RESTART_POD_NAME" 120; then
        echo "❌ Test failed: Restart pod did not become ready after previous kill with signal ${SIGNAL}"
        delete_pod "$RESTART_POD_NAME"
        return 1
    fi
    
    # Step 6: Clean up recovery pod
    echo "6. Restart pod became ready successfully. Deleting pod..."
    delete_pod "$RESTART_POD_NAME"
    if ! wait_for_pod_deletion "$RESTART_POD_NAME" 60; then
        echo "⚠️ Warning: Pod deletion timed out"
        return 1
    fi

    # Step 7: Test recovery with runc runtime
    echo "7. Deploying test pod with runc runtime to verify system restart..."
    deploy_test_pod "runc" "$IMAGE_URL" "$RESTART_POD_NAME"
    if ! wait_for_pod "$RESTART_POD_NAME" 120; then
        echo "❌ Test failed: Restart pod with runc did not become ready"
        delete_pod "$RESTART_POD_NAME"
        return 1
    fi
    
    # Step 8: Clean up runc recovery pod
    echo "8. Runc restart pod became ready successfully. Deleting pod..."
    delete_pod "$RESTART_POD_NAME"
    if ! wait_for_pod_deletion "$RESTART_POD_NAME" 60; then
        echo "⚠️ Warning: Pod deletion timed out"
        return 1
    fi
    
    echo "✅ Stability test with signal ${SIGNAL} (${SIGNAL_NAME}) passed successfully"
    return 0
}

# Run all signal tests and collect results
run_all_tests() {
    local SIGNALS=(
        "2:SIGINT (Ctrl+C)"
        "15:SIGTERM (graceful termination)"
        "9:SIGKILL (force kill)"
    )
    
    for signal_info in "${SIGNALS[@]}"; do
        IFS=':' read -r signal name <<< "$signal_info"
        if ! test_kill_signal "$signal" "$name"; then
            return 1
        fi
    done
    
    return 0
}

if run_all_tests; then
    print_header "STABILITY TESTS: PASSED ✅"
else
    print_header "STABILITY TESTS: FAILED ❌"
    exit 1
fi

