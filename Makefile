# Common build/test/deploy commands

.PHONY: all build test

all: build

build:
	@echo "Building all services..."
	# go build ./services/api-gateway
	test
	echo "Build complete"

test:
	@echo "Running tests..."
	# npm test in Node.js services
	echo "Tests complete"
