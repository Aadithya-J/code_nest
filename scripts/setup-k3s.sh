#!/bin/bash
set -e

echo "🚀 Setting up k3s for CodeNest Workspace Orchestration"

# Check if k3s is already installed
if command -v k3s &> /dev/null; then
    echo "✅ k3s already installed"
    k3s --version
else
    echo "📦 Installing k3s..."
    # Install k3s with minimal components
    # --disable=traefik: We don't need the ingress controller (using our own API gateway)
    # --disable=servicelb: We don't need load balancer (single node)
    # --disable=local-storage: We'll use hostPath for simplicity
    curl -sfL https://get.k3s.io | INSTALL_K3S_EXEC="--disable=traefik --disable=servicelb" sh -
    
    echo "✅ k3s installed successfully"
fi

# Wait for k3s to be ready
echo "⏳ Waiting for k3s to be ready..."
sudo k3s kubectl wait --for=condition=Ready nodes --all --timeout=60s

# Create kubeconfig for runner-allocator service
echo "🔑 Setting up kubeconfig for runner-allocator..."
sudo cp /etc/rancher/k3s/k3s.yaml /tmp/k3s-config.yaml
sudo chmod 644 /tmp/k3s-config.yaml

# Replace localhost with the actual IP for container access
# In production, this would be the droplet's internal IP
# For local dev, we'll use host.docker.internal or the actual IP
if [[ "$OSTYPE" == "darwin"* ]]; then
    # macOS - use host.docker.internal
    sed -i '' 's/127.0.0.1/host.docker.internal/g' /tmp/k3s-config.yaml
else
    # Linux - use the actual IP
    LOCAL_IP=$(hostname -I | awk '{print $1}')
    sed -i "s/127.0.0.1/$LOCAL_IP/g" /tmp/k3s-config.yaml
fi

echo "📋 Creating workspace namespace..."
sudo k3s kubectl create namespace workspaces --dry-run=client -o yaml | sudo k3s kubectl apply -f -

echo "🎯 Setting up RBAC for runner-allocator..."
cat <<EOF | sudo k3s kubectl apply -f -
apiVersion: v1
kind: ServiceAccount
metadata:
  name: runner-allocator
  namespace: default
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: runner-allocator
rules:
- apiGroups: [""]
  resources: ["pods", "services"]
  verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
- apiGroups: [""]
  resources: ["pods/log", "pods/exec"]
  verbs: ["get", "create"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: runner-allocator
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: runner-allocator
subjects:
- kind: ServiceAccount
  name: runner-allocator
  namespace: default
EOF

echo "✅ k3s setup complete!"
echo "📍 Kubeconfig available at: /tmp/k3s-config.yaml"
echo "🔍 Check cluster status: sudo k3s kubectl get nodes"
echo "🎯 Test workspace namespace: sudo k3s kubectl get pods -n workspaces"
