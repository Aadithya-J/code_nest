# Common build/test/deploy commands

.PHONY: all build test run

all: proto build

run:
	@echo "Starting all services..."
	@./bin/auth-service & ./bin/api-gateway &
	@echo "Services are running."

proto:
	@echo "Generating gRPC code from proto files..."
	protoc -I proto proto/auth.proto \
		--go_out=proto --go-grpc_out=proto

build: auth api-gateway

auth:
	@echo "Building auth-service..."
	go build -o bin/auth-service services/auth-service/cmd/auth-service/main.go

api-gateway:
	@echo "Building api-gateway..."
	go build -o bin/api-gateway services/api-gateway/cmd/api-gateway/main.go

test:
	@echo "Running tests..."
	# add test commands here
	echo "Tests complete"
