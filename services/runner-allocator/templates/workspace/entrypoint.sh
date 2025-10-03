#!/bin/bash
set -e

# Source NVM for Node.js
export NVM_DIR="/home/node/.nvm"
[ -s "$NVM_DIR/nvm.sh" ] && \. "$NVM_DIR/nvm.sh"

# Environment variables
PROJECT_ID=${PROJECT_ID:-"default"}
GIT_REPO_URL=${GIT_REPO_URL:-""}

echo "🚀 Starting workspace for project: $PROJECT_ID"

# Clone repository if provided
if [ -n "$GIT_REPO_URL" ]; then
    echo "📦 Cloning repository: $GIT_REPO_URL"
    cd /workspace
    if git clone "$GIT_REPO_URL" project 2>/dev/null; then
        cd project
        echo "✅ Repository cloned successfully"
        
        # Auto-detect and install dependencies
        if [ -f "package.json" ]; then
            echo "📦 Installing npm dependencies..."
            npm install
        fi
        
        if [ -f "requirements.txt" ]; then
            echo "📦 Installing Python dependencies..."
            pip3 install -r requirements.txt
        fi
        
        if [ -f "go.mod" ]; then
            echo "📦 Downloading Go dependencies..."
            go mod download
        fi
    else
        echo "⚠️  Failed to clone repository, starting with empty workspace"
    fi
else
    echo "📝 Starting with empty workspace"
fi

# Start ttyd (web terminal) in background
echo "🖥️  Starting web terminal on port 7681..."
ttyd -p 7681 -W /workspace bash &

# Keep container running
echo "✅ Workspace ready!"
tail -f /dev/null
