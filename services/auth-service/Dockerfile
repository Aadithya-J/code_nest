# syntax=docker/dockerfile:1.4
# Stage 1: Builder
FROM golang:1.24-alpine AS builder

# Set workdir at repo root
WORKDIR /app

# Copy entire repository
COPY . .

# Enter service directory
WORKDIR /app/services/auth-service

# Use BuildKit cache mounts for much faster builds
# This caches the Go module cache and build cache across builds
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go mod download

# Build with cache mounts
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build -o /auth-service ./cmd/auth-service

# Stage 2: Final
FROM gcr.io/distroless/static-debian11

COPY --from=builder /auth-service /

# gRPC services typically run on port 50051
EXPOSE 50051

ENTRYPOINT ["/auth-service"]
