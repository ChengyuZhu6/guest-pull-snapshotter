#!/bin/bash
set -e

CONTAINERD_VERSION=${CONTAINERD_VERSION:-"1.7.26"}

echo "Installing containerd ${CONTAINERD_VERSION}"

curl -fsSL https://github.com/containerd/containerd/releases/download/v${CONTAINERD_VERSION}/containerd-${CONTAINERD_VERSION}-linux-amd64.tar.gz | tar -xz -C /usr/local

cat > /etc/systemd/system/containerd.service << EOF
[Unit]
Description=containerd container runtime
Documentation=https://containerd.io
After=network.target local-fs.target

[Service]
ExecStartPre=-/sbin/modprobe overlay
ExecStart=/usr/local/bin/containerd
Restart=always
RestartSec=5
Delegate=yes
KillMode=process
OOMScoreAdjust=-999
LimitNOFILE=1048576
LimitNPROC=infinity
LimitCORE=infinity

[Install]
WantedBy=multi-user.target
EOF

mkdir -p /etc/containerd
containerd config default > /etc/containerd/config.toml

sed -i 's/disabled_plugins = \["cri"\]/enabled_plugins = \["cri"\]/' /etc/containerd/config.toml

# Check the path pattern in the configuration file
if grep -q "plugins.'io.containerd.cri.v1.runtime'" /etc/containerd/config.toml; then
    # containerd 2.0+ configuration path
    OPTIONS_PATH="plugins.'io.containerd.cri.v1.runtime'.containerd.runtimes.runc.options"
elif grep -q "plugins.\"io.containerd.grpc.v1.cri\"" /etc/containerd/config.toml; then
    # containerd 1.x configuration path
    OPTIONS_PATH="plugins.\"io.containerd.grpc.v1.cri\".containerd.runtimes.runc.options"
else
    echo "Cannot recognize the structure of containerd configuration file"
    exit 1
fi

# Configure SystemdCgroup setting
configure_systemd_cgroup() {
    # If SystemdCgroup = false exists, change it to true
    if grep -q "SystemdCgroup = false" /etc/containerd/config.toml; then
        sed -i 's/SystemdCgroup = false/SystemdCgroup = true/' /etc/containerd/config.toml
        echo "Changed SystemdCgroup from false to true"
        return
    fi

    # In containerd 2.0, the SystemdCgroup setting does not exist in the config file, we need to add it to the correct section
    if ! grep -q "SystemdCgroup" /etc/containerd/config.toml; then
        # First find the last line of the options section
        LAST_OPTION_LINE=$(grep -n "\[$OPTIONS_PATH\]" -A 20 /etc/containerd/config.toml | grep -v "\[$OPTIONS_PATH\]" | grep -m 1 -B 1 "^\s*\[" | head -n 1 | cut -d'-' -f1)

        if [ -n "$LAST_OPTION_LINE" ]; then
            # Insert SystemdCgroup setting before the last line of the options section
            sed -i "${LAST_OPTION_LINE}i\\            SystemdCgroup = true" /etc/containerd/config.toml
            echo "Added SystemdCgroup = true before line ${LAST_OPTION_LINE}"
        else
            # If the end of the options section can't be found, try to add after the beginning of the options section
            OPTIONS_LINE=$(grep -n "\[$OPTIONS_PATH\]" /etc/containerd/config.toml | cut -d':' -f1)
            if [ -n "$OPTIONS_LINE" ]; then
                sed -i "${OPTIONS_LINE}a\\            SystemdCgroup = true" /etc/containerd/config.toml
                echo "Added SystemdCgroup = true after line ${OPTIONS_LINE}"
            else
                echo "Cannot find [$OPTIONS_PATH] section, unable to add SystemdCgroup setting"
                return 1
            fi
        fi
    fi
}

configure_systemd_cgroup

cat /etc/containerd/config.toml

systemctl daemon-reload
systemctl enable --now containerd
systemctl restart containerd

mkdir -p /opt/cni/bin
curl -fsSL https://github.com/containernetworking/plugins/releases/download/v1.4.0/cni-plugins-linux-amd64-v1.4.0.tgz | tar -xz -C /opt/cni/bin

echo "Containerd installation completed"
systemctl status containerd --no-pager
