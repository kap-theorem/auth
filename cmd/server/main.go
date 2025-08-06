package main

import (
	"context"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"authservice/pkg/service"
	authv1 "authservice/proto/auth/v1"

	"google.golang.org/grpc"
)

func main() {
	listen, err := net.Listen("tcp", ":8080")
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}

	grpcserver := grpc.NewServer()
	authv1.RegisterAuthServiceServer(grpcserver, service.NewAuthServiceServer())

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