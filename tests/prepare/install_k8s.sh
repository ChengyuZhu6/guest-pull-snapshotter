#!/bin/bash

set -e

K8S_VERSION=${K8S_VERSION:-"1.29.2"}
FLANNEL_VERSION=${FLANNEL_VERSION:-"v0.24.0"}
POD_NETWORK_CIDR=${POD_NETWORK_CIDR:-"10.244.0.0/16"}

echo "Installing Kubernetes ${K8S_VERSION} with Flannel ${FLANNEL_VERSION}"

sudo swapoff -a
sudo sed -i '/swap/s/^/#/' /etc/fstab

cat <<EOF | sudo tee /etc/modules-load.d/k8s.conf
overlay
br_netfilter
kvm
EOF

sudo modprobe overlay
sudo modprobe br_netfilter

cat <<EOF | sudo tee /etc/sysctl.d/k8s.conf
net.bridge.bridge-nf-call-iptables  = 1
net.bridge.bridge-nf-call-ip6tables = 1
net.ipv4.ip_forward                 = 1
EOF

sudo sysctl --system

mkdir -p /etc/apt/keyrings
curl -fsSL https://pkgs.k8s.io/core:/stable:/v${K8S_VERSION%.*}/deb/Release.key | gpg --batch --yes --dearmor -o /etc/apt/keyrings/kubernetes-apt-keyring.gpg
echo "deb [signed-by=/etc/apt/keyrings/kubernetes-apt-keyring.gpg] https://pkgs.k8s.io/core:/stable:/v${K8S_VERSION%.*}/deb/ /" | tee /etc/apt/sources.list.d/kubernetes.list

apt-get update

if apt-cache madison kubelet | grep -q "${K8S_VERSION}"; then
  apt-get install -y --allow-downgrades kubelet=${K8S_VERSION}-* kubeadm=${K8S_VERSION}-* kubectl=${K8S_VERSION}-*
else
  echo "Specific version ${K8S_VERSION} not found, installing latest available version"
  apt-get install -y --allow-downgrades kubelet kubeadm kubectl
fi

apt-mark hold kubelet kubeadm kubectl

echo "Installed Kubernetes versions:"
kubelet --version
kubeadm version
kubectl version --client

cat <<EOF | sudo tee /etc/crictl.yaml
runtime-endpoint: unix:///run/containerd/containerd.sock
image-endpoint: unix:///run/containerd/containerd.sock
timeout: 10
debug: false
EOF

echo "Initializing Kubernetes control plane"
sudo kubeadm config images pull --kubernetes-version=${K8S_VERSION}
sudo kubeadm init --pod-network-cidr=${POD_NETWORK_CIDR} --kubernetes-version=${K8S_VERSION}
export KUBECONFIG=/etc/kubernetes/admin.conf
    
echo "Installing Flannel ${FLANNEL_VERSION}"
kubectl apply -f https://github.com/flannel-io/flannel/releases/download/${FLANNEL_VERSION}/kube-flannel.yml

kubectl taint nodes --all node.kubernetes.io/control-plane- || true
kubectl taint nodes --all node-role.kubernetes.io/control-plane- || true
kubectl taint nodes --all node-role.kubernetes.io/master- || true
kubectl taint nodes --all node.kubernetes.io/master- || true

NODENAME=$(kubectl get nodes -o jsonpath='{.items[0].metadata.name}')
kubectl label node ${NODENAME} node.kubernetes.io/worker='' --overwrite

check_cluster_ready() {
    echo "Waiting for node to be Ready..."
    
    TIMEOUT=300
    INTERVAL=10
    ELAPSED=0
    
    while [ $ELAPSED -lt $TIMEOUT ]; do
        NODE_STATUS=$(kubectl get nodes -o jsonpath='{.items[0].status.conditions[?(@.type=="Ready")].status}')
        if [ "$NODE_STATUS" == "True" ]; then
            echo "Node is Ready!"
            break
        fi
        echo "Node not ready yet, waiting... ($ELAPSED/$TIMEOUT seconds)"
        sleep $INTERVAL
        ELAPSED=$((ELAPSED + INTERVAL))
    done
    
    if [ $ELAPSED -ge $TIMEOUT ]; then
        echo "Timeout waiting for node to be ready"
        kubectl get nodes
        exit 1
    fi
    
    echo "Waiting for all pods to be Running and Ready..."
    
    TIMEOUT=600
    ELAPSED=0
    
    while [ $ELAPSED -lt $TIMEOUT ]; do
        if kubectl wait --for=condition=Ready pods --all --all-namespaces --timeout=1s >/dev/null 2>&1; then
            echo "All pods are Ready!"
            break
        fi
        
        echo "Some pods are not ready yet, waiting... ($ELAPSED/$TIMEOUT seconds)"
        kubectl get pods --all-namespaces
        sleep $INTERVAL
        ELAPSED=$((ELAPSED + INTERVAL))
    done
    
    if [ $ELAPSED -ge $TIMEOUT ]; then
        echo "Timeout waiting for pods to be ready"
        kubectl get pods --all-namespaces
        exit 1
    fi
    
    echo "Cluster status:"
    kubectl get nodes
    kubectl get pods --all-namespaces
    
    return 0
}

check_cluster_ready

echo "Kubernetes installation completed"
kubectl version --client
echo "Kubelet status:"
sudo systemctl status kubelet --no-pager

echo "Kubernetes and Flannel installation completed successfully"
