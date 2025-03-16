#!/bin/bash
set -e

echo "Installing custom Kata Containers runtime"

# Clone the repository
KATA_REPO="https://github.com/kata-containers/kata-containers.git"
KATA_BRANCH="main"
KATA_DIR="/tmp/kata-containers"

# Clean up any previous installation
rm -rf "$KATA_DIR"

# Clone the repository
echo "Cloning Kata Containers repository from $KATA_REPO (branch: $KATA_BRANCH)"
git clone "$KATA_REPO" -b "$KATA_BRANCH" "$KATA_DIR"

# Apply the patch
echo "Applying guest-pull snapshotter patch"
cp tests/kata-patch/0001-support-guest-pull-snapshotter.patch "$KATA_DIR"
cd "$KATA_DIR"
git apply 0001-support-guest-pull-snapshotter.patch

# Build and install the runtime
echo "Building Kata Containers runtime"
cd "$KATA_DIR/src/runtime"
make

echo "Installing Kata Containers runtime to /opt/kata/bin/"
mkdir -p /opt/kata/bin/
cp containerd-shim-kata-v2 /opt/kata/bin/

echo "Custom Kata Containers runtime installation completed"

