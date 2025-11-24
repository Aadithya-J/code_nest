#!/bin/bash
set -e

echo "🔧 Fixing K3s Docker Connectivity for Runner-Allocator"
echo "======================================================"

# Get the actual k3d server IP
echo "📍 Detecting k3d cluster network..."
SERVER_IP=$(docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' k3d-codenest-server-0)

if [ -z "$SERVER_IP" ]; then
    echo "❌ Could not find k3d-codenest-server-0 IP"
    echo "   Run: docker network inspect k3d-codenest"
    exit 1
fi

echo "✅ Found k3d-codenest-server-0 at: $SERVER_IP"

# Create docker-accessible kubeconfig
echo ""
echo "🔑 Creating Docker-accessible kubeconfig..."
k3d kubeconfig get codenest | \
    sed "s/0.0.0.0:[0-9]*/k3d-codenest-server-0:6443/" > \
    /tmp/k3s-config-docker.yaml

chmod 644 /tmp/k3s-config-docker.yaml
echo "✅ Created /tmp/k3s-config-docker.yaml"

# Update docker-compose.yml with correct IP
echo ""
echo "📝 Updating docker-compose.yml..."
sed -i.bak "s/k3d-codenest-server-0:[0-9.]\+/k3d-codenest-server-0:$SERVER_IP/" docker-compose.yml
echo "✅ Updated extra_hosts with IP: $SERVER_IP"

# Verify the change
echo ""
echo "🔍 Verification:"
echo "   Server IP: $SERVER_IP"
echo "   Kubeconfig server: $(grep 'server:' /tmp/k3s-config-docker.yaml | awk '{print $2}')"
echo "   docker-compose entry: $(grep 'k3d-codenest-server-0' docker-compose.yml | xargs)"

echo ""
echo "✅ Fix complete! Now run:"
echo "   docker-compose up -d runner-allocator"
echo "   docker logs runner-allocator --follow"
echo ""
echo "Expected output:"
echo "   ✅ Successfully connected to Kubernetes cluster"
echo "   ✅ Created slot 1/2/3"
