# syntax=docker/dockerfile:1.4
FROM golang:1.24-alpine AS builder

WORKDIR /app

# ---------- Workspace-aware build ----------
# Copy minimal files first to leverage Docker layer cache
COPY proto/ proto/
COPY services/workspace-service/go.* services/workspace-service/

# Download deps (cached unless go.* change)
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    cd services/workspace-service && go mod download

# Now copy the actual source
COPY services/workspace-service services/workspace-service

# Build binary
WORKDIR /app/services/workspace-service
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /workspace-service ./cmd/workspace-service

FROM alpine:latest
COPY --from=builder /workspace-service /workspace-service
RUN apk --no-cache add ca-certificates

EXPOSE 50052
ENTRYPOINT ["/workspace-service"]
