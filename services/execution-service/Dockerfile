# Stage 1: Builder
FROM golang:1.24-alpine AS builder

# Set workdir at repo root
WORKDIR /app

# Copy entire repository
COPY . .

# Enter service directory
WORKDIR /app/services/execution-service

RUN go mod download

# Build
RUN CGO_ENABLED=0 GOOS=linux go build -o /execution-service ./cmd/execution-service

# Stage 2: Final
FROM gcr.io/distroless/static-debian11

COPY --from=builder /execution-service /

EXPOSE 50053

ENTRYPOINT ["/execution-service"]
