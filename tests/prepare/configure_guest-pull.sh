#!/bin/bash
set -e

echo "Configuring guest pull in containerd"

CONTAINERD_CONFIG="/etc/containerd/config.toml"

if grep -q "disable_snapshot_annotations" "$CONTAINERD_CONFIG"; then
    sed -i 's/disable_snapshot_annotations = true/disable_snapshot_annotations = false/g' "$CONTAINERD_CONFIG"
    echo "Changed disable_snapshot_annotations from true to false"
else
    echo "Warning: disable_snapshot_annotations setting not found in config"
fi

if grep -q "\[proxy_plugins\]" "$CONTAINERD_CONFIG"; then
    sed -i '/\[proxy_plugins\]/a \
\[proxy_plugins.guest-pull\]\n      type = "snapshot"\n      address = "\/run\/containerd-guest-pull-grpc\/containerd-guest-pull-grpc.sock"' "$CONTAINERD_CONFIG"
else
    echo -e "\n[proxy_plugins]\n[proxy_plugins.guest-pull]\n      type = \"snapshot\"\n      address = \"/run/containerd-guest-pull-grpc/containerd-guest-pull-grpc.sock\"" >> "$CONTAINERD_CONFIG"
fi

if grep -q 'snapshotter = "nydus"' "$CONTAINERD_CONFIG"; then
    sed -i 's/snapshotter = "nydus"/snapshotter = "guest-pull"/g' "$CONTAINERD_CONFIG"
    echo "Changed snapshotter from nydus to guest-pull"
else
    echo "Warning: snapshotter = \"nydus\" setting not found in config"
fi

sed -i 's/level = ""/level = "debug"/g' "$CONTAINERD_CONFIG"
sed -i 's/import = [*]/import = []/g' "$CONTAINERD_CONFIG"

    
echo "Successfully added guest-pull plugin configuration to containerd config"

cat /etc/containerd/config.toml

echo "run guest-pull snapshotter"

sudo containerd-guest-pull-grpc > /tmp/containerd-guest-pull-grpc.log --log-level debug 2>&1 & 

systemctl restart containerd

echo "Containerd has been configured with guest-pull plugin and restarted"

ctr plugin ls|grep snapshotter
