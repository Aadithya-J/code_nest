# Stage 1: Builder
FROM golang:1.21-alpine AS builder
WORKDIR /app

# Copy go module manifests & download deps first for better layer caching
COPY go.mod go.sum ./
RUN go mod download

# Copy the rest of the source code
COPY . .

# Build
RUN CGO_ENABLED=0 GOOS=linux go build -o /execution-service ./cmd/execution-service

# Stage 2: Minimal runtime image
FROM gcr.io/distroless/static-debian11
COPY --from=builder /execution-service /execution-service
EXPOSE 50053
ENTRYPOINT ["/execution-service"]
