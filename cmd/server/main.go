package main

import (
	"context"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	database "authservice/internal/database"
	"authservice/internal/httpgateway"
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

	isDev := strings.EqualFold(os.Getenv("ENV"), "dev")

	dbConnection := database.GetDBConnection()
	dbConnection.CreateTables()

	// Phase 2 bootstrap: built-in platform org/client, then superadmin
	// tuples for SUPERADMIN_EMAILS that match existing developers.
	bootstrapCtx, bootstrapCancel := context.WithTimeout(context.Background(), 30*time.Second)
	if err := service.BootstrapPlatform(bootstrapCtx, dbConnection.DB); err != nil {
		log.Fatalf("Failed to bootstrap platform client: %v", err)
	}
	if err := service.BootstrapSuperadmins(bootstrapCtx, dbConnection.DB); err != nil {
		log.Fatalf("Failed to bootstrap superadmins: %v", err)
	}
	bootstrapCancel()

	// Start cleanup service in background
	cleanupService := service.NewCleanupService(dbConnection.DB)
	cleanupService.Start()

	rateLimiter := ratelimit.NewRateLimiter()

	serverOpts := []grpc.ServerOption{
		grpc.ChainUnaryInterceptor(loggingInterceptor, rateLimiter.UnaryInterceptor()),
	}

	// TLS: required unless ENV=dev (applies to both listeners)
	certFile := os.Getenv("TLS_CERT_FILE")
	keyFile := os.Getenv("TLS_KEY_FILE")
	tlsEnabled := certFile != "" && keyFile != ""
	if tlsEnabled {
		creds, err := credentials.NewServerTLSFromFile(certFile, keyFile)
		if err != nil {
			log.Fatalf("Failed to load TLS credentials: %v", err)
		}
		serverOpts = append(serverOpts, grpc.Creds(creds))
		log.Println("TLS enabled on listener")
	} else if isDev {
		log.Println("WARNING: ENV=dev, serving plaintext (no TLS)")
	} else {
		log.Fatal("TLS_CERT_FILE and TLS_KEY_FILE must be set (plaintext is only allowed when ENV=dev)")
	}

	authService := service.NewAuthServiceServer(dbConnection.DB)
	platformService := service.NewPlatformServiceServer(dbConnection.DB)

	grpcserver := grpc.NewServer(serverOpts...)
	authv1.RegisterAuthServiceServer(grpcserver, authService)
	authv1.RegisterPlatformServiceServer(grpcserver, platformService)

	// Enable reflection for grpcurl
	reflection.Register(grpcserver)

	go func() {
		log.Println("Starting the server on: 8080")
		if err := grpcserver.Serve(listen); err != nil {
			log.Fatalf("Failed to start server")
		}
	}()

	// HTTP JSON gateway for the browser console: same interceptors, same
	// TLS rules, permissive CORS in dev (the console proxies to it in dev).
	httpPort := os.Getenv("HTTP_PORT")
	if httpPort == "" {
		httpPort = "8081"
	}
	gateway := httpgateway.New(
		chainInterceptors(loggingInterceptor, rateLimiter.UnaryInterceptor()),
		isDev,
		httpgateway.Service{Desc: &authv1.AuthService_ServiceDesc, Impl: authService},
		httpgateway.Service{Desc: &authv1.PlatformService_ServiceDesc, Impl: platformService},
	)
	httpServer := &http.Server{
		Addr:              ":" + httpPort,
		Handler:           gateway,
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		log.Printf("Starting the HTTP gateway on: %s", httpPort)
		var err error
		if tlsEnabled {
			err = httpServer.ListenAndServeTLS(certFile, keyFile)
		} else {
			err = httpServer.ListenAndServe()
		}
		if err != nil && err != http.ErrServerClosed {
			log.Fatalf("Failed to start HTTP gateway: %v", err)
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

	// Stop the HTTP gateway
	if err := httpServer.Shutdown(ctx); err != nil {
		log.Printf("HTTP gateway shutdown error: %v", err)
	}

	// Stop background jobs
	cleanupService.Stop()

	// Close database after gRPC server stops accepting new connections
	dbConnection.Close()
}

// chainInterceptors composes unary interceptors for the HTTP gateway (the
// gRPC server uses grpc.ChainUnaryInterceptor natively).
func chainInterceptors(interceptors ...grpc.UnaryServerInterceptor) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		chained := handler
		for i := len(interceptors) - 1; i >= 0; i-- {
			next, ic := chained, interceptors[i]
			chained = func(ctx context.Context, req interface{}) (interface{}, error) {
				return ic(ctx, req, info, next)
			}
		}
		return chained(ctx, req)
	}
}

func loggingInterceptor(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	start := time.Now()

	log.Printf("[RPC START] Method: %s, Time: %s", info.FullMethod, start.Format(time.RFC3339))

	// Call the handler to proceed with the RPC
	resp, err := handler(ctx, req)

	log.Printf("[RPC END] Method: %s, Duration: %s, Error: %v", info.FullMethod, time.Since(start), err)
	return resp, err
}
