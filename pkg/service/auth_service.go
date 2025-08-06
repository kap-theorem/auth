package service

import (
	authv1 "authservice/proto/auth/v1"
	"context"

	"google.golang.org/protobuf/types/known/emptypb"
)

type AuthServiceServer struct{
	authv1.UnimplementedAuthServiceServer
}

func NewAuthServiceServer() *AuthServiceServer {
	return &AuthServiceServer{}
}

func (s *AuthServiceServer) HealthCheck(ctx context.Context, in *emptypb.Empty) (*authv1.HealthCheckResponse, error) {
	return &authv1.HealthCheckResponse{
		Status: authv1.HealthCheckResponse_SERVING,
		Message: "Auth Server is running",
		Details: map[string]string{
			"version": "1.0.0",
			"status": "healthy",
		},
	}, nil
}