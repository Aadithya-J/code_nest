# syntax=docker/dockerfile:1.4
# Stage 1 - builder
FROM golang:1.24-alpine AS builder

WORKDIR /app

# Copy entire repository (needed for Go workspace with local dependencies)
COPY . .

# Enter service directory
WORKDIR /app/services/api-gateway

# Use BuildKit cache mounts for faster builds
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go mod download

# Build with cache mounts
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /api-gateway ./cmd/api-gateway

# Stage 2 - final (minimal)
FROM gcr.io/distroless/static-debian11

# copy binary from builder
COPY --from=builder /api-gateway /api-gateway

EXPOSE 8080

ENTRYPOINT ["/api-gateway"]
