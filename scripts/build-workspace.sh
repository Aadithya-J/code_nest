#!/bin/bash
set -e

echo "🏗️  Building workspace image..."
docker build -t workspace:latest services/runner-allocator/templates/workspace

echo "📦 Importing image into k3d cluster 'codenest'..."
k3d image import workspace:latest -c codenest

echo "✅ Workspace image updated and imported!"
