FROM ubuntu:22.04

# Prevent interactive prompts during build
ENV DEBIAN_FRONTEND=noninteractive

# Install system dependencies, including Python and Java
RUN apt-get update && apt-get install -y --no-install-recommends \
    git \
    curl \
    wget \
    ca-certificates \
    tzdata \
    build-essential \
    python3 \
    python3-pip \
    python3-venv \
    inotify-tools \
    jq \
    && rm -rf /var/lib/apt/lists/*

# ---------- Install Node.js via nvm ----------
ENV NVM_DIR=/home/node/.nvm
ENV NODE_VERSION=18
# Create a non-root user
RUN useradd -m -s /bin/bash node
USER node
RUN curl -o- https://raw.githubusercontent.com/nvm-sh/nvm/v0.39.5/install.sh | bash

# Set NVM environment variables for future shell sessions
ENV NODE_PATH $NVM_DIR/versions/node/v$NODE_VERSION/lib/node_modules
ENV PATH $NVM_DIR/versions/node/v$NODE_VERSION/bin:$PATH

RUN . $NVM_DIR/nvm.sh && nvm install $NODE_VERSION && nvm alias default $NODE_VERSION && nvm use default

# ---------- Switch back to root for global installs ----------
USER root

# ---------- Install Go ----------
ENV GOROOT=/usr/local/go
ENV GOPATH=/workspace/go
ENV PATH=$PATH:$GOROOT/bin:$GOPATH/bin
RUN curl -L https://go.dev/dl/go1.22.2.linux-amd64.tar.gz | tar -C /usr/local -xz

# ---------- Install ttyd (web terminal) ----------
RUN wget -qO /usr/local/bin/ttyd https://github.com/tsl0922/ttyd/releases/download/1.7.3/ttyd.x86_64 \
    && chmod +x /usr/local/bin/ttyd

# Create and set permissions for workspace directory
WORKDIR /workspace
RUN chown -R node:node /workspace

# Copy entrypoint script and set permissions (as root)
COPY entrypoint.sh /home/node/entrypoint.sh
RUN chmod +x /home/node/entrypoint.sh && chown node:node /home/node/entrypoint.sh

# Switch to non-root user
USER node

# Expose dev-server and terminal ports
EXPOSE 3000 7681

# Run the entrypoint script
CMD ["/home/node/entrypoint.sh"]
