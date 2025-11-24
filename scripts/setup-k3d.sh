#!/bin/bash
set -e

echo "🚀 Setting up k3d (k3s in Docker) for local CodeNest development"

# Check if k3d is installed
if ! command -v k3d &> /dev/null; then
    echo "📦 Installing k3d..."
    if [[ "$OSTYPE" == "darwin"* ]]; then
        # macOS
        if command -v brew &> /dev/null; then
            brew install k3d
        else
            curl -s https://raw.githubusercontent.com/k3d-io/k3d/main/install.sh | bash
        fi
    else
        # Linux
        curl -s https://raw.githubusercontent.com/k3d-io/k3d/main/install.sh | bash
    fi
    echo "✅ k3d installed successfully"
else
    echo "✅ k3d already installed"
    k3d version
fi

# Create k3d cluster for CodeNest
CLUSTER_NAME="codenest"

# Check if cluster already exists
if k3d cluster list | grep -q "$CLUSTER_NAME"; then
    echo "✅ k3d cluster '$CLUSTER_NAME' already exists"
else
    echo "🏗️ Creating k3d cluster '$CLUSTER_NAME'..."
    k3d cluster create $CLUSTER_NAME \
        --port "6443:6443@loadbalancer" \
        --port "80:80@loadbalancer" \
        --port "443:443@loadbalancer" \
        --k3s-arg "--disable=traefik@server:0" \
        --k3s-arg "--disable=servicelb@server:0"
    
    echo "✅ k3d cluster created successfully"
fi

# Set kubectl context
echo "🔧 Setting kubectl context..."
k3d kubeconfig merge $CLUSTER_NAME --kubeconfig-switch-context

# Wait for cluster to be ready
echo "⏳ Waiting for cluster to be ready..."
kubectl wait --for=condition=Ready nodes --all --timeout=60s

# Create workspaces namespace
echo "📋 Creating workspaces namespace..."
kubectl create namespace workspaces --dry-run=client -o yaml | kubectl apply -f -

# Set up RBAC for runner-allocator
echo "🎯 Setting up RBAC for runner-allocator..."
cat <<EOF | kubectl apply -f -
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

# Export kubeconfig for runner-allocator
echo "🔑 Exporting kubeconfig for runner-allocator..."
k3d kubeconfig get $CLUSTER_NAME > /tmp/k3s-config.yaml
chmod 644 /tmp/k3s-config.yaml

echo "✅ k3d setup complete!"
echo "📍 Kubeconfig available at: /tmp/k3s-config.yaml"
echo "🔍 Check cluster status: kubectl get nodes"
echo "🎯 Test workspaces namespace: kubectl get pods -n workspaces"
echo "🗑️  To cleanup: k3d cluster delete $CLUSTER_NAME"
