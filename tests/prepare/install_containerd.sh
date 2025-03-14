#!/bin/bash
set -e

CONTAINERD_VERSION=${CONTAINERD_VERSION:-"1.7.26"}

echo "Installing containerd version ${CONTAINERD_VERSION}"

ARCH=$(uname -m)
case $ARCH in
    x86_64)
        GOARCH="amd64"
        ;;
    aarch64|arm64)
        GOARCH="arm64"
        ;;
    *)
        echo "Unsupported architecture: $ARCH"
        exit 1
        ;;
esac

sudo apt-get remove -y containerd containerd.io containerd-io || true
sudo rm -rf /etc/containerd /var/lib/containerd

CONTAINERD_URL="https://github.com/containerd/containerd/releases/download/v${CONTAINERD_VERSION}/containerd-${CONTAINERD_VERSION}-linux-${GOARCH}.tar.gz"
echo "Downloading containerd from: ${CONTAINERD_URL}"

TMP_DIR=$(mktemp -d)
curl -sL "${CONTAINERD_URL}" -o "${TMP_DIR}/containerd.tar.gz"
sudo tar Cxzvf /usr/local "${TMP_DIR}/containerd.tar.gz"
rm -rf "${TMP_DIR}"

sudo mkdir -p /etc/containerd

sudo containerd config default | sed -e 's/\(SystemdCgroup =\).*/\1 true/g' | tee /etc/containerd/config.toml

cat << EOF | sudo tee /etc/systemd/system/containerd.service > /dev/null
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

sudo systemctl daemon-reload
sudo systemctl enable --now containerd

echo "Containerd version:"
containerd --version

echo "Containerd installation completed successfully"
