FROM golang:1.24-alpine AS builder

WORKDIR /app

RUN apk add --no-cache git build-base

COPY proto/ proto/
COPY services/project-service/go.* services/project-service/
RUN --mount=type=cache,target=/go/pkg/mod \
    cd services/project-service && go mod download

COPY services/project-service services/project-service

WORKDIR /app/services/project-service
RUN --mount=type=cache,target=/go/pkg/mod \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /app/project-service ./cmd/project-service

FROM gcr.io/distroless/static-debian12

COPY --from=builder /app/project-service /app/project-service

EXPOSE 50052
ENTRYPOINT ["/app/project-service"]
