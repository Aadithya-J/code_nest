# syntax=docker/dockerfile:1.4
# Stage 1: Builder
FROM golang:1.24-alpine AS builder

# Set workdir at repo root
WORKDIR /app

# Copy minimal files first to leverage Docker layer cache
COPY proto/ proto/
COPY services/auth-service/go.* services/auth-service/

# Download deps (cached unless go.* change)
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    cd services/auth-service && go mod download

# Now copy the actual source
COPY services/auth-service services/auth-service
WORKDIR /app/services/auth-service

# Build with cache mounts
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build -o /auth-service ./cmd/auth-service

# Stage 2: Final
FROM gcr.io/distroless/static-debian12

COPY --from=builder /auth-service /

# gRPC services typically run on port 50051
EXPOSE 50051

ENTRYPOINT ["/auth-service"]
