# syntax=docker/dockerfile:1.4
# Stage 1 - builder
FROM golang:1.24-alpine AS builder

WORKDIR /app

# Copy minimal files first to leverage Docker layer cache
COPY proto/ proto/
COPY services/api-gateway/go.* services/api-gateway/

# Download deps (cached unless go.* change)
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    cd services/api-gateway && go mod download

# Now copy the actual source
COPY services/api-gateway services/api-gateway
WORKDIR /app/services/api-gateway

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
