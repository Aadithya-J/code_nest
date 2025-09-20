# syntax=docker/dockerfile:1.4
FROM golang:1.24-alpine AS builder

WORKDIR /app

# Copy entire repository
COPY . .

# Enter service directory
WORKDIR /app/services/workspace-service

# Use BuildKit cache mounts for faster builds
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go mod download

# Build with cache mounts
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build -o /workspace-service ./cmd/workspace-service .

FROM gcr.io/distroless/static-debian11
COPY --from=builder /workspace-service /workspace-service

EXPOSE 50052
ENTRYPOINT ["/workspace-service"]
