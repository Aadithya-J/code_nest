# Stage 1: Builder
# Use the official Golang image to build the application
FROM golang:1.24-alpine AS builder

# Set the working directory inside the container
# Build using repo root context
WORKDIR /app

COPY . .

WORKDIR /app/services/api-gateway

RUN go mod download

RUN CGO_ENABLED=0 GOOS=linux go build -o /api-gateway ./cmd/api-gateway

# Stage 2: Final
# Use a minimal, non-root image for the final container
FROM gcr.io/distroless/static-debian11

# Copy the built binary from the builder stage
COPY --from=builder /api-gateway /

# Expose the port the API Gateway will run on
EXPOSE 8080

# Set the entrypoint for the container
ENTRYPOINT ["/api-gateway"]
