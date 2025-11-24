#!/bin/bash
set -e

echo "🧹 Cleaning up workspace slots for testing..."

# Delete all pods and services in workspaces namespace
echo "📦 Deleting all workspace pods and services..."
kubectl delete pods --all -n workspaces --ignore-not-found=true
kubectl delete services --all -n workspaces --ignore-not-found=true

echo "⏳ Waiting for pods to terminate..."
kubectl wait --for=delete pod --all -n workspaces --timeout=60s || true

# Restart runner-allocator to reinitialize slots
echo "🔄 Restarting runner-allocator..."
docker-compose restart runner-allocator

# Wait for slots to be ready
echo "⏳ Waiting for slots to initialize..."
sleep 15

echo "✅ Cleanup complete! Slots are ready for testing."
