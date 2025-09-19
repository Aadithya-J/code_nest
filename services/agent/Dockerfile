# Stage 1: Builder
FROM golang:1.21-alpine AS builder
WORKDIR /app

# Caching dependencies first
COPY go.mod go.sum ./
RUN go mod download

# Copy source
COPY . .

# Build the agent binary
RUN CGO_ENABLED=0 GOOS=linux go build -o /agent ./cmd/agent

# Stage 2: Distroless runtime
FROM gcr.io/distroless/static-debian11
COPY --from=builder /agent /agent
EXPOSE 4000
ENTRYPOINT ["/agent"]
