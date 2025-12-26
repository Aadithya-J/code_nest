# syntax=docker/dockerfile:1.4

FROM golang:1.24-alpine AS builder

WORKDIR /app

# Copy dependency manifests first for caching
COPY proto/ proto/
COPY services/gateway/go.* services/gateway/

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    cd services/gateway && go mod download

# Copy the rest of the source
COPY services/gateway services/gateway
WORKDIR /app/services/gateway

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build -o /gateway ./cmd/gateway

FROM gcr.io/distroless/static-debian12

COPY --from=builder /gateway /

EXPOSE 3000

ENTRYPOINT ["/gateway"]
