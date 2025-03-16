# Guest Pull Snapshotter

A containerd snapshotter plugin that enables container images to be pulled directly inside Kata Containers guest VMs, improving security and isolation.

## Overview

Guest Pull Snapshotter is a specialized containerd snapshotter that works with Kata Containers to enable container images to be pulled directly inside the guest VM rather than on the host. This approach provides several benefits:

- **Enhanced Security**: Container images are never extracted on the host, reducing the attack surface
- **Improved Isolation**: Image content remains isolated within the VM boundary
- **Compatibility**: Works seamlessly with both Kata Containers and standard runc runtime

## Architecture

The Guest Pull Snapshotter consists of two main components:

1. **Snapshotter Service (`containerd-guest-pull-grpc`)**: A gRPC service that implements the containerd snapshotter interface and communicates with containerd
2. **Mount Helper (`guest-pull-overlayfs`)**: A utility that handles the mounting of overlay filesystems to avoid handling the image on the host.

When a container is started with Kata Containers runtime, the snapshotter intercepts image mount requests and passes special volume information to the Kata runtime, which then pulls and mounts the image inside the guest VM.

## Prerequisites

- Kubernetes 1.24+
- containerd 1.7+
- Kata Containers 3.0+
- Confidential Containers (CoCo)

## Installation

### 1. Install the Guest Pull Snapshotter

```bash
# Clone the repository
git clone https://github.com/ChengyuZhu6/guest-pull-snapshotter.git
cd guest-pull-snapshotter

# Build and install
make
sudo make install
```

### 2. Enable and start the guest-pull-snapshotter service

```bash
# Configure and enable the service
sudo ./tests/prepare/enable_guest-pull_service.sh
```

### 3. Configure containerd

Update your containerd configuration (`/etc/containerd/config.toml`) to use the guest-pull snapshotter:

```toml
[proxy_plugins]
  [proxy_plugins.guest-pull]
    type = "snapshot"
    address = "/run/containerd-guest-pull-grpc/containerd-guest-pull-grpc.sock"
[plugins."io.containerd.grpc.v1.cri".containerd]
    disable_snapshot_annotations = false
[plugins."io.containerd.grpc.v1.cri".containerd.runtimes.kata-qemu-coco-dev]
    snapshotter = "guest-pull"
```

### 4. Install Confidential Containers (CoCo)

```bash
# Install CoCo components
sudo ./tests/prepare/install_coco.sh
```

### 5. Install patched Kata Containers runtime

The Guest Pull Snapshotter requires a patched version of Kata Containers:

```bash
# Install patched Kata Containers
sudo ./tests/prepare/install_patched_kata_runtime.sh
```

## Usage

Once installed and configured, the Guest Pull Snapshotter works transparently with Kata Containers. You can deploy pods using the `kata-qemu-coco-dev` runtime class:

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: nginx-kata
spec:
  runtimeClassName: kata-qemu-coco-dev
  containers:
  - name: nginx
    image: nginx:latest
```

## Configuration

The Guest Pull Snapshotter can be configured using command-line flags or environment variables:

| Flag | Environment Variable | Default | Description |
|------|---------------------|---------|-------------|
| `--address` | `GUEST_PULL_ADDRESS` | `/run/containerd-guest-pull-grpc/containerd-guest-pull-grpc.sock` | Socket path for the gRPC server |
| `--config` | `GUEST_PULL_CONFIG` | `/etc/containerd-guest-pull-grpc/config.toml` | Path to configuration file |
| `--log-level` | `GUEST_PULL_LOG_LEVEL` | `info` | Logging level |
| `--root` | `GUEST_PULL_ROOT` | `/var/lib/containerd-guest-pull-grpc` | Root directory for the snapshotter |

## Testing

The project includes comprehensive test suites to verify functionality:

```bash
# Run all tests
make test

# Run specific test suites
sudo ./tests/test-cases/functional.sh
sudo ./tests/test-cases/compatibility.sh
sudo ./tests/test-cases/stability.sh
```

### Test Suites

1. **Functional Tests**: Verify the core functionality of guest image pulling
2. **Compatibility Tests**: Verify that both runc and kata-qemu-coco-dev runtimes can successfully run containers with various images
3. **Stability Tests**: Verify system stability with various signals to the guest-pull snapshotter service

## Troubleshooting

### Common Issues

1. **Pods stuck in ContainerCreating state**:
   - Check snapshotter logs: `journalctl -u guest-pull-snapshotter`
   - Check containerd configuration: `cat /etc/containerd/config.toml`

2. **Service not starting**:
   - Check service status: `systemctl status guest-pull-snapshotter`
   - Verify installation: `ls -la /usr/local/bin/containerd-guest-pull-grpc`

3. **Image pull failures**:
   - Check if the guest-pull service is running: `systemctl status guest-pull-snapshotter`
   - Verify the image exists in the registry and is accessible

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

## License

This project is licensed under the Apache License 2.0 - see the LICENSE file for details.