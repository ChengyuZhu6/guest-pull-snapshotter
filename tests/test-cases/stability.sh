#!/bin/bash
set -e

# Source common test utilities
source tests/utils/test_utils.sh

echo "Running stabilization tests"

# Run a complete test cycle for a specific signal
test_kill_signal() {
    local SIGNAL=$1
    local SIGNAL_NAME=$2
    local RUNTIME="kata-qemu-coco-dev"
    local IMAGE_NAME="nginx"
    local POD_NAME="stability-test-signal-${SIGNAL}"
    local RECOVERY_POD_NAME="${POD_NAME}-recovery"
    local IMAGE_URL=${TEST_IMAGES[$IMAGE_NAME]}
    
    echo "=== Testing stability with kill signal ${SIGNAL} (${SIGNAL_NAME}) ==="
    
    # Step 1: Deploy initial pod
    echo "1. Deploying initial test pod..."
    deploy_test_pod "$RUNTIME" "$IMAGE_URL" "$POD_NAME"
    sleep 5  # Wait for pod to start
    
    # Step 2: Kill the process
    if ! kill_guest_pull_process "$SIGNAL"; then
        delete_pod "$POD_NAME"
        return 1
    fi
    
    # Step 3: Check if pod can still become ready
    echo "3. Checking if pod can still become ready after kill..."
    if ! wait_for_pod "$POD_NAME" 120; then
        echo "❌ Test failed: Pod did not become ready after killing process with signal ${SIGNAL}"
        kubectl describe pod "$POD_NAME"
        delete_pod "$POD_NAME"
        return 1
    fi
    
    # Step 4: Clean up first pod
    echo "4. Pod became ready successfully after kill. Deleting pod..."
    delete_pod "$POD_NAME"
    if ! wait_for_pod_deletion "$POD_NAME" 60; then
        echo "❌ Warning: Pod deletion timed out"
        return 1
    fi
    
    # Step 5: Test recovery with a second pod
    echo "5. Deploying second test pod to verify system recovery..."
    deploy_test_pod "$RUNTIME" "$IMAGE_URL" "$RECOVERY_POD_NAME"
    if ! wait_for_pod "$RECOVERY_POD_NAME" 120; then
        echo "❌ Test failed: Recovery pod did not become ready after previous kill with signal ${SIGNAL}"
        kubectl describe pod "$RECOVERY_POD_NAME"
        delete_pod "$RECOVERY_POD_NAME"
        return 1
    fi
    
    # Step 6: Clean up recovery pod
    echo "6. Recovery pod became ready successfully. Deleting pod..."
    delete_pod "$RECOVERY_POD_NAME"
    if ! wait_for_pod_deletion "$RECOVERY_POD_NAME" 60; then
        echo "❌ Warning: Pod deletion timed out"
        return 1
    fi
    
    echo "✅ Stability test with signal ${SIGNAL} (${SIGNAL_NAME}) passed successfully"
    return 0
}

# Run all signal tests and collect results
run_all_tests() {
    local FAILED=0
    local SIGNALS=(
        "2:SIGINT (Ctrl+C)"
        "15:SIGTERM (graceful termination)"
        "9:SIGKILL (force kill)"
    )
    
    for signal_info in "${SIGNALS[@]}"; do
        IFS=':' read -r signal name <<< "$signal_info"
        if ! test_kill_signal "$signal" "$name"; then
            FAILED=1
        fi
    done
    
    return $FAILED
}

# Main execution
if run_all_tests; then
    echo "✅ All stability tests passed successfully"
else
    echo "❌ Some stability tests failed"
    exit 1
fi

echo "Cleaning up test resources..."

