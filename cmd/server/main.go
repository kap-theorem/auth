package main

import (
	"context"
	"log"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	database "authservice/internal/database"
	"authservice/pkg/ratelimit"
	"authservice/pkg/service"
	authv1 "authservice/proto/auth/v1"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/reflection"
)

func main() {
	listen, err := net.Listen("tcp", ":8080")
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}

	dbConnection := database.GetDBConnection()
	dbConnection.CreateTables()

	// Start cleanup service in background
	cleanupService := service.NewCleanupService(dbConnection.DB)
	cleanupService.Start()

	rateLimiter := ratelimit.NewRateLimiter()

	serverOpts := []grpc.ServerOption{
		grpc.ChainUnaryInterceptor(loggingInterceptor, rateLimiter.UnaryInterceptor()),
	}

	// TLS: required unless ENV=dev
	certFile := os.Getenv("TLS_CERT_FILE")
	keyFile := os.Getenv("TLS_KEY_FILE")
	if certFile != "" && keyFile != "" {
		creds, err := credentials.NewServerTLSFromFile(certFile, keyFile)
		if err != nil {
			log.Fatalf("Failed to load TLS credentials: %v", err)
		}
		serverOpts = append(serverOpts, grpc.Creds(creds))
		log.Println("TLS enabled on listener")
	} else if strings.EqualFold(os.Getenv("ENV"), "dev") {
		log.Println("WARNING: ENV=dev, serving plaintext (no TLS)")
	} else {
		log.Fatal("TLS_CERT_FILE and TLS_KEY_FILE must be set (plaintext is only allowed when ENV=dev)")
	}

	grpcserver := grpc.NewServer(serverOpts...)
	authv1.RegisterAuthServiceServer(grpcserver, service.NewAuthServiceServer(dbConnection.DB))

	// Enable reflection for grpcurl
	reflection.Register(grpcserver)

	go func() {
		log.Println("Starting the server on: 8080")
		if err := grpcserver.Serve(listen); err != nil {
			log.Fatalf("Failed to start server")
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	<-quit

	log.Println("Shutting down server")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	done := make(chan struct{})
	go func() {
		grpcserver.GracefulStop()
		close(done)
	}()

	select {
	case <-done:
		log.Println("Server stopped gracefully")
	case <-ctx.Done():
		log.Println("Shutdown timeout exceeded, forcing stop")
		grpcserver.Stop()
	}

	// Stop background jobs
	cleanupService.Stop()

	// Close database after gRPC server stops accepting new connections
	dbConnection.Close()
}

func loggingInterceptor(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	start := time.Now()

	log.Printf("[RPC START] Method: %s, Time: %s", info.FullMethod, start.Format(time.RFC3339))

	// Call the handler to proceed with the RPC
	resp, err := handler(ctx, req)

	log.Printf("[RPC END] Method: %s, Duration: %s, Error: %v", info.FullMethod, time.Since(start), err)
	return resp, err
}
