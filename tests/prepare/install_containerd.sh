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

sed -i 's/SystemdCgroup = false/SystemdCgroup = true/' /etc/containerd/config.toml

cat /etc/containerd/config.toml

systemctl daemon-reload
systemctl enable --now containerd
systemctl restart containerd

mkdir -p /opt/cni/bin
curl -fsSL https://github.com/containernetworking/plugins/releases/download/v1.4.0/cni-plugins-linux-amd64-v1.4.0.tgz | tar -xz -C /opt/cni/bin

echo "Containerd installation completed"
systemctl status containerd --no-pager
