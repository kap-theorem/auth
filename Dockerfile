# Build stage
FROM golang:1.24-alpine AS builder

WORKDIR /app

# Install dependencies
COPY go.mod go.sum ./
RUN go mod download

# Install protobuf compiler and Go plugins
RUN apk add --no-cache protobuf protobuf-dev
RUN go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
RUN go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

# Copy source code
COPY . .

# Generate protobuf files
RUN protoc -I . -I /usr/include --go_out=. --go-grpc_out=. proto/auth/v1/auth.proto

# Build the application
RUN go build -o bin/auth-server ./cmd/server/

# Runtime stage
FROM alpine:latest

RUN apk --no-cache add ca-certificates
WORKDIR /root/

# Copy the binary from builder stage
COPY --from=builder /app/bin/auth-server .

# Expose port
EXPOSE 8080

# Run the binary
CMD ["./auth-server"]
