#!/bin/bash
set -o errexit

# 1. Create a kind cluster
cat <<EOF | kind create cluster --config=-
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
- role: control-plane
  kubeadmConfigPatches:
  - | 
    kind: InitConfiguration
    nodeRegistration:
      kubeletExtraArgs:
        node-labels: "ingress-ready=true"
  extraPortMappings:
  - containerPort: 80
    hostPort: 80
    protocol: TCP
  - containerPort: 443
    hostPort: 443
    protocol: TCP
EOF

# 2. Install Ingress NGINX
kubectl apply -f https://raw.githubusercontent.com/kubernetes/ingress-nginx/main/deploy/static/provider/kind/deploy.yaml
kubectl wait --namespace ingress-nginx \
  --for=condition=ready pod \
  --selector=app.kubernetes.io/component=controller \
  --timeout=90s

# 3. Load locally built service images into the Kind cluster
IMAGES=(
  api-gateway
  auth-service
  workspace-service
  execution-service
  agent
)
for img in "${IMAGES[@]}"; do
  if docker images | grep -q "${img}"; then
    echo "Loading $img into Kind..."
    kind load docker-image "${img}:latest"
  else
    echo "[WARN] Image $img not found locally; build it first if needed."
  fi
done

echo "Kind cluster ready. Local images loaded."
