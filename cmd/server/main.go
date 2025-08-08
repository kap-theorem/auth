package main

import (
	"context"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	database "authservice/internal/database"
	"authservice/pkg/service"
	authv1 "authservice/proto/auth/v1"

	"google.golang.org/grpc"
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
	go cleanupService.StartCleanupJob()

	grpcserver := grpc.NewServer(
		grpc.UnaryInterceptor(unaryInterceptor),
	)
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
}

func unaryInterceptor(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	start := time.Now()

	log.Printf("[RPC START] Method: %s, Time: %s", info.FullMethod, start.Format(time.RFC3339))

	// Call the handler to proceed with the RPC
	resp, err := handler(ctx, req)

	log.Printf("[RPC END] Method: %s, Duration: %s, Error: %v", info.FullMethod, time.Since(start), err)
	return resp, err
}
