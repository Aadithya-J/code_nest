# Build stage
FROM golang:1.24-alpine AS builder

WORKDIR /app

# Copy proto files first
COPY proto/ proto/

# Copy runner-allocator go mod files
COPY services/runner-allocator/go.mod services/runner-allocator/go.sum services/runner-allocator/

# Download dependencies
WORKDIR /app/services/runner-allocator
RUN go mod download

# Copy source code
COPY services/runner-allocator/ .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -o runner-allocator cmd/runner-allocator/main.go

# Runtime stage
FROM alpine:latest

# Install docker CLI for Docker-in-Docker mode
RUN apk --no-cache add docker-cli ca-certificates

WORKDIR /app

# Copy the binary
COPY --from=builder /app/services/runner-allocator/runner-allocator .

# Expose port
EXPOSE 50053

# Run the application
CMD ["./runner-allocator"]
