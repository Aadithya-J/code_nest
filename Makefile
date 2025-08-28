# Common build/test/deploy commands

.PHONY: all build test run proto

all: proto build

run:
	@echo "Starting all services..."
	@./bin/auth-service & ./bin/api-gateway & ./bin/workspace-service &
	@echo "Services are running."

proto:
	@echo "Generating gRPC code from proto files..."
	protoc --go_out=proto --go-grpc_out=proto --proto_path=proto proto/auth.proto proto/workspace.proto

build: auth api-gateway workspace

auth:
	@echo "Building auth-service..."
	go build -o bin/auth-service services/auth-service/cmd/auth-service/main.go

api-gateway:
	@echo "Building api-gateway..."
	go build -o bin/api-gateway services/api-gateway/cmd/api-gateway/main.go

workspace:
	@echo "Building workspace-service..."
	go build -o bin/workspace-service services/workspace-service/cmd/workspace-service/main.go

test:
	@echo "Running tests..."
	# add test commands here
	echo "Tests complete"

testrun:
	@echo "Running all services in test mode..."
	@(cd services/api-gateway && go run cmd/api-gateway/main.go &) \
	 && (cd services/auth-service && go run cmd/auth-service/main.go &) \
	 && (cd services/workspace-service && go run cmd/workspace-service/main.go &)
	@echo "Ran all"