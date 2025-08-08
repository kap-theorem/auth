.PHONY: build run clean proto test

# Build the server
build:
	go build -o ./bin/auth-server ./cmd/server/

# Run the server
run:
	go run ./cmd/server/main.go

# Clean build artifacts
clean:
	rm -rf ./bin/

# Generate protobuf files
proto:
	protoc --go_out=. --go-grpc_out=. proto/auth/v1/auth.proto

# Run tests
test:
	go test ./...

# Install dependencies
deps:
	go mod tidy
	go mod download

# Format code
fmt:
	go fmt ./...

# Lint code
lint:
	golangci-lint run

# Docker build
docker-build:
	docker build -t auth-service .

# Help
help:
	@echo "Available commands:"
	@echo "  build       - Build the server binary"
	@echo "  run         - Run the server"
	@echo "  clean       - Clean build artifacts"
	@echo "  proto       - Generate protobuf files"
	@echo "  test        - Run tests"
	@echo "  deps        - Install dependencies"
	@echo "  fmt         - Format code"
	@echo "  lint        - Lint code"
	@echo "  docker-build - Build Docker image"
