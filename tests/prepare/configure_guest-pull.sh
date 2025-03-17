#!/bin/bash
set -e

CONTAINERD_VERSION=${CONTAINERD_VERSION:-"1.7.26"}
CONTAINERD_MAJOR_VERSION=$(echo $CONTAINERD_VERSION | cut -d. -f1)
CONTAINERD_CONFIG="/etc/containerd/config.toml"

echo "Configuring guest pull in containerd"

# Function to update or add a setting in config.toml
update_config() {
    local SETTING=$1
    local OLD_VALUE=$2
    local NEW_VALUE=$3
    local SECTION=$4
    
    if grep -q "$SETTING = $OLD_VALUE" "$CONTAINERD_CONFIG"; then
        sed -i "s/$SETTING = $OLD_VALUE/$SETTING = $NEW_VALUE/g" "$CONTAINERD_CONFIG"
        echo "Changed $SETTING from $OLD_VALUE to $NEW_VALUE"
    elif [ -n "$SECTION" ] && ! grep -q "$SETTING = " "$CONTAINERD_CONFIG"; then
        # Add setting to specified section if it doesn't exist
        if grep -q "\[$SECTION\]" "$CONTAINERD_CONFIG"; then
            sed -i "/\[$SECTION\]/a \\$SETTING = $NEW_VALUE" "$CONTAINERD_CONFIG"
            echo "Added $SETTING = $NEW_VALUE to [$SECTION] section"
        else
            echo "Warning: [$SECTION] section not found in config"
        fi
    else
        echo "Warning: $SETTING setting not found in config"
    fi
}

# Update snapshot annotations setting
update_config "disable_snapshot_annotations" "true" "false"

# Add guest-pull plugin configuration
if ! grep -q "\[proxy_plugins.guest-pull\]" "$CONTAINERD_CONFIG"; then
    if grep -q "\[proxy_plugins\]" "$CONTAINERD_CONFIG"; then
        sed -i '/\[proxy_plugins\]/a \
\[proxy_plugins.guest-pull\]\n      type = "snapshot"\n      address = "\/run\/containerd-guest-pull-grpc\/containerd-guest-pull-grpc.sock"' "$CONTAINERD_CONFIG"
    else
        echo -e "\n[proxy_plugins]\n[proxy_plugins.guest-pull]\n      type = \"snapshot\"\n      address = \"/run/containerd-guest-pull-grpc/containerd-guest-pull-grpc.sock\"" >> "$CONTAINERD_CONFIG"
    fi
    echo "Added guest-pull plugin configuration"
fi

# Update the snapshotter for kata runtime
if [[ "$CONTAINERD_MAJOR_VERSION" -ge 2 ]]; then
    # Used for containerd 2.0+
    # Replace snapshotter in kata-deploy.toml
    sed -i 's/snapshotter = "nydus"/snapshotter = "guest-pull"/g' "/opt/kata/containerd/config.d/kata-deploy.toml"

    # Also update the cri image plugin's snapshotter for kata runtime
    echo -e '\n[plugins."io.containerd.cri.v1.images".runtime_platforms.kata-qemu-coco-dev]\nsnapshotter = "guest-pull"' >> "$CONTAINERD_CONFIG"
    echo "Added CRI plugin configuration with guest-pull snapshotter"

    cat /opt/kata/containerd/config.d/kata-deploy.toml
else
    # Used for containerd 1.7.x
    update_config "snapshotter" "\"nydus\"" "\"guest-pull\""
fi

# Update other settings
sed -i 's/level = ""/level = "debug"/g' "$CONTAINERD_CONFIG"
sed -i 's/import = \[*]/import = \[\]/g' "$CONTAINERD_CONFIG"

echo "Successfully configured guest-pull plugin in containerd"

# Restart containerd to apply changes
systemctl restart containerd
echo "Containerd has been configured with guest-pull plugin and restarted"
cat /etc/containerd/config.toml
# Verify plugin is loaded
ctr plugin ls | grep snapshotter
