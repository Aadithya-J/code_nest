#!/bin/bash
set -e

# Source NVM for Node.js
export NVM_DIR="/home/node/.nvm"
[ -s "$NVM_DIR/nvm.sh" ] && \. "$NVM_DIR/nvm.sh"

# Environment variables
PROJECT_ID=${PROJECT_ID:-"default"}
SESSION_ID=${SESSION_ID:-""}
GIT_REPO_URL=${GIT_REPO_URL:-""}
GITHUB_TOKEN=${GITHUB_TOKEN:-""}
RABBITMQ_URL=${RABBITMQ_URL:-""}
TARGET_BRANCH=${TARGET_BRANCH:-"main"}

echo "🚀 Starting workspace for project: $PROJECT_ID (Session: $SESSION_ID)"

# Function to publish file events to RabbitMQ
publish_event() {
    local event_type=$1
    local file_path=$2
    
    if [ -n "$RABBITMQ_URL" ]; then
        # TODO: Use amqp-publish or curl to RabbitMQ HTTP API
        # For now, log it
        echo "📡 Event: $event_type - $file_path"
    fi
}

# Function to auto-commit changes
auto_commit() {
    cd /workspace/project
    
    # Check if there are any changes
    if ! git diff --quiet || ! git diff --cached --quiet; then
        git add -A
        git commit -m "Auto-save: $(date -u +%Y-%m-%dT%H:%M:%SZ)" || true
        
        # Push to temp branch
        if [ -n "$GITHUB_TOKEN" ]; then
            git push origin "thinide-session/$SESSION_ID" 2>/dev/null || true
        fi
    fi
}

# Clone repository if provided
if [ -n "$GIT_REPO_URL" ]; then
    echo "📦 Cloning repository: $GIT_REPO_URL"
    cd /workspace
    
    # Configure git to use token for authentication
    if [ -n "$GITHUB_TOKEN" ]; then
        GIT_URL_WITH_TOKEN=$(echo "$GIT_REPO_URL" | sed "s|https://|https://$GITHUB_TOKEN@|")
        git clone "$GIT_URL_WITH_TOKEN" project 2>/dev/null || git clone "$GIT_REPO_URL" project
    else
        git clone "$GIT_REPO_URL" project
    fi
    
    if [ -d "project" ]; then
        cd project
        echo "✅ Repository cloned successfully"
        
        # Configure git user
        git config user.name "CodeNest Workspace"
        git config user.email "workspace@codenest.dev"
        
        # Create and checkout temp branch
        TEMP_BRANCH="thinide-session/$SESSION_ID"
        git checkout -b "$TEMP_BRANCH"
        
        if [ -n "$GITHUB_TOKEN" ]; then
            # Push temp branch to remote
            git push -u origin "$TEMP_BRANCH" 2>/dev/null || true
        fi
        
        echo "🌿 Created temp branch: $TEMP_BRANCH"
        
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
        
        # Start inotify watcher in background for auto-commit
        if [ -n "$SESSION_ID" ]; then
            echo "👁️  Starting file watcher..."
            (
                while inotifywait -r -e modify,create,delete,move /workspace/project 2>/dev/null; do
                    sleep 2  # Debounce: wait 2 seconds for more changes
                    auto_commit
                done
            ) &
            INOTIFY_PID=$!
            echo "✅ File watcher started (PID: $INOTIFY_PID)"
        fi
    else
        echo "⚠️  Failed to clone repository, starting with empty workspace"
    fi
else
    echo "📝 Starting with empty workspace"
fi

# Start ttyd (web terminal) in background
echo "🖥️  Starting web terminal on port 7681..."
ttyd -p 7681 -w /workspace bash &

# Keep container running
echo "✅ Workspace ready!"
tail -f /dev/null
