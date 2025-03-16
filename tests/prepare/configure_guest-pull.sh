#!/bin/bash
set -e

echo "Configuring guest pull in containerd"

CONTAINERD_CONFIG="/etc/containerd/config.toml"

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

# Update snapshotter setting
update_config "snapshotter" "\"nydus\"" "\"guest-pull\""

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
