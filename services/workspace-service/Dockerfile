# Stage 1: Builder
FROM golang:1.24-alpine AS builder

# Set workdir at repo root copy
WORKDIR /app

# Copy entire repository (context is repo root)
COPY . .

# Move into service directory
WORKDIR /app/services/workspace-service

# Download dependencies
RUN go mod download

# Build binary
RUN CGO_ENABLED=0 GOOS=linux go build -o /workspace-service ./cmd/workspace-service

# Stage 2: Final
FROM gcr.io/distroless/static-debian11

COPY --from=builder /workspace-service /

# gRPC services typically run on port 50051, but we'll use 50052 to avoid conflicts
EXPOSE 50052

ENTRYPOINT ["/workspace-service"]
