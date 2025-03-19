#!/bin/bash
#
# Private Registry Test Suite for Guest Pull Snapshotter
# =====================================================
#
# Purpose:
#   This test verifies that the guest-pull snapshotter can properly
#   pull images from private registries (GitHub Container Registry).
#
# Test Strategy:
#   1. Use GitHub Container Registry as the private registry
#   2. Create Kubernetes secret for registry credentials
#   3. Deploy a pod using the private registry image with kata-qemu-coco-dev runtime
#   4. Verify the pod becomes failed due to authorization error in the guest VM
#
# Expected Results:
#   - Pod should fail to start with the private registry image due to authorization in guest
#   - The specific authorization failure should be found in the containerd logs
#

set -e

source tests/utils/test_utils.sh

PRIVATE_IMAGE="ghcr.io/chengyuzhu6/redis:auth"

# Record timestamp at the beginning of the test
TEST_START_TIME=$(date +"%Y-%m-%d %H:%M:%S")

check_containerd_auth_error() {
    echo "Checking containerd logs for authorization errors..."

    # Retrieve and filter logs from journalctl
    if journalctl -u containerd --since "$TEST_START_TIME" | grep -qE 'failed to pull manifest Not authorized'; then
        echo "✅ Found expected authorization error in containerd logs"
        return 0
    else
        echo "❌ Authorization error not found in containerd logs"
        echo "=== Containerd logs snippet ==="
        journalctl -u containerd --since "$TEST_START_TIME"
        echo "==============================="
        return 1
    fi
}

# Setup GitHub Container Registry credentials
setup_ghcr_credentials() {
    echo "Setting up GitHub Container Registry credentials..."
    if [ -z "${GITHUB_TOKEN}" ] && [ -z "${GITHUB_ACTOR}" ]; then
        # Handle local environment credentials
        if [ -z "${CI}" ]; then
            echo "Running in local environment, please provide GitHub credentials"
            read -p "GitHub Username: " GITHUB_USER
            read -sp "GitHub Token: " GITHUB_TOKEN
            echo
        else
            echo "Error: GitHub credentials not available in CI environment"
            return 1
        fi
    else
        # Use CI environment credentials
        GITHUB_USER="${GITHUB_ACTOR}"
    fi

    kubectl create secret docker-registry cococred \
        --docker-server="ghcr.io" \
        --docker-username="${GITHUB_USER}" \
        --docker-password="${GITHUB_TOKEN}"
    kubectl get secret cococred
    echo "GitHub Container Registry credentials setup completed"
    return 0
}

# Test private registry image pulling in k8s
test_private_registry() {
    local RUNTIME="$1"
    echo "Testing private registry image pulling with runtime: ${RUNTIME}"

    local POD_NAME="private-registry-test"

    # Create pod with pull secret
    cat <<EOF | kubectl apply -f -
apiVersion: v1
kind: Pod
metadata:
  name: ${POD_NAME}
  annotations:
    io.containerd.cri.runtime-handler: ${RUNTIME}
spec:
  runtimeClassName: ${RUNTIME}
  containers:
  - name: test-container
    image: ${PRIVATE_IMAGE}
  imagePullSecrets:
  - name: cococred
EOF

    if wait_for_pod "${POD_NAME}" 120; then
        echo "❌ The pod unexpectedly succeeded to start with private registry image!"
        kubectl delete pod "${POD_NAME}"
        return 1
    fi
    
    echo "Pod failed to start as expected, verifying failure reason..."
    
    # Verify failure reasons: should be an authorization error and the image should not be pulled on the host
    local auth_error_found=false
    local image_integrity_verified=false
    
    # Check for authorization errors
    if check_containerd_auth_error; then
        auth_error_found=true
        echo "✅ Found expected authorization error in containerd logs"
    else
        echo "❌ No authorization error found in containerd logs"
    fi
    
    # Check image integrity using the common function
    if verify_image_integrity "${PRIVATE_IMAGE}"; then
        image_integrity_verified=true
    fi
    
    # Clean up resources
    kubectl delete pod "${POD_NAME}"
    
    # Test passes only when both conditions are met
    if $auth_error_found && $image_integrity_verified; then
        echo "✅ Test passed: Pod failed with authorization error and image not pulled on host"
        return 0
    else
        echo "❌ Test failed: Pod failed but not for the expected reasons"
        return 1
    fi
}

run_authentication_tests() {
    echo "Testing guest-pull functionality with private registry images from GitHub Container Registry"

    # Setup GitHub Container Registry credentials
    if ! setup_ghcr_credentials; then
        echo "Failed to setup GitHub Container Registry credentials"
        return 1
    fi

    # Run test with private registry image
    if ! test_private_registry "kata-qemu-coco-dev"; then
        return 1
    fi

    return 0
}

run_test_suite "PRIVATE REGISTRY TESTS" run_authentication_tests || exit 1
