#!/bin/bash

# Simple make replacement for Git Bash
# Usage: ./make.sh <target>

case "$1" in
    "build")
        echo "Building the server..."
        go build -o ./bin/auth-server ./cmd/server/
        ;;
    "run")
        echo "Running the server..."
        go run ./cmd/server/main.go
        ;;
    "clean")
        echo "Cleaning build artifacts..."
        rm -rf ./bin/
        ;;
    "proto")
        echo "Generating protobuf files..."
        protoc --go_out=. --go-grpc_out=. proto/auth/v1/auth.proto
        ;;
    "proto-win")
        echo "Generating protobuf files..."
        protoc --go_out=. --go-grpc_out=. proto/auth/v1/auth.proto
        ;;
    "test")
        echo "Running tests..."
        go test ./...
        ;;
    "deps")
        echo "Installing dependencies..."
        go mod tidy
        go mod download
        ;;
    "fmt")
        echo "Formatting code..."
        go fmt ./...
        ;;
    "lint")
        echo "Linting code..."
        golangci-lint run
        ;;
    "docker-build")
        echo "Building Docker image..."
        docker build -t auth-service .
        ;;
    "help"|"")
        echo "Available commands:"
        echo "  build       - Build the server binary"
        echo "  run         - Run the server"
        echo "  clean       - Clean build artifacts"
        echo "  proto       - Generate protobuf files"
        echo "  proto-win   - Generate protobuf files (Windows)"
        echo "  test        - Run tests"
        echo "  deps        - Install dependencies"
        echo "  fmt         - Format code"
        echo "  lint        - Lint code"
        echo "  docker-build - Build Docker image"
        echo "  help        - Show this help message"
        ;;
    *)
        echo "Unknown target: $1"
        echo "Run './make.sh help' to see available commands"
        exit 1
        ;;
esac
