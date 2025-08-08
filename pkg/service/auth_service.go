package service

import (
	"authservice/pkg/models"
	"authservice/pkg/repository"
	"authservice/pkg/utils"
	authv1 "authservice/proto/auth/v1"
	"context"
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"

	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
	"gorm.io/gorm"
)

type AuthServiceServerImpl struct {
	authv1.UnimplementedAuthServiceServer
	repo *repository.AuthRepository
}

func NewAuthServiceServer(db *gorm.DB) *AuthServiceServerImpl {
	return &AuthServiceServerImpl{
		repo: repository.NewAuthRepository(db),
	}
}

func (s *AuthServiceServerImpl) HealthCheck(ctx context.Context, in *emptypb.Empty) (*authv1.HealthCheckResponse, error) {
	return &authv1.HealthCheckResponse{
		Status:  authv1.HealthCheckResponse_SERVING,
		Message: "Auth Server is running",
		Details: map[string]string{
			"version": "1.0.0",
			"status":  "healthy",
		},
	}, nil
}

func (s *AuthServiceServerImpl) RegisterUser(ctx context.Context, req *authv1.RegisterUserRequest) (*authv1.RegisterUserResponse, error) {
	log.Printf("RegisterUser request received for email: %s", req.Email)

	// Validation
	if err := s.validateUserRegistration(req); err != nil {
		return &authv1.RegisterUserResponse{
			Success: false,
			Message: err.Error(),
		}, nil
	}

	// Check if client exists
	clientExists, err := s.repo.IsClientExists(req.ClientId)
	if err != nil {
		log.Printf("Error checking client existence: %v", err)
		return &authv1.RegisterUserResponse{
			Success: false,
			Message: "Internal server error",
		}, nil
	}
	if !clientExists {
		return &authv1.RegisterUserResponse{
			Success: false,
			Message: "Invalid client ID",
		}, nil
	}

	// Check if email already exists
	emailExists, err := s.repo.IsEmailExists(req.Email)
	if err != nil {
		log.Printf("Error checking email existence: %v", err)
		return &authv1.RegisterUserResponse{
			Success: false,
			Message: "Internal server error",
		}, nil
	}
	if emailExists {
		return &authv1.RegisterUserResponse{
			Success: false,
			Message: "Email already registered",
		}, nil
	}

	// Hash password
	hashedPassword, err := utils.HashPassword(req.Password)
	if err != nil {
		log.Printf("Error hashing password: %v", err)
		return &authv1.RegisterUserResponse{
			Success: false,
			Message: "Internal server error",
		}, nil
	}

	// Create user
	userID := utils.GenerateUUID()
	user := &models.User{
		UserId:   userID,
		UserName: req.Username,
		EmailId:  req.Email,
		Password: hashedPassword,
		ClientId: req.ClientId,
	}

	if err := s.repo.CreateUser(user); err != nil {
		log.Printf("Error creating user: %v", err)
		return &authv1.RegisterUserResponse{
			Success: false,
			Message: "Failed to create user",
		}, nil
	}

	log.Printf("User registered successfully: %s", userID)
	return &authv1.RegisterUserResponse{
		Success: true,
		Message: "User registered successfully",
		UserId:  userID,
	}, nil
}

func (s *AuthServiceServerImpl) LoginUser(ctx context.Context, req *authv1.LoginUserRequest) (*authv1.LoginUserResponse, error) {
	log.Printf("LoginUser request received for email: %s", req.Email)

	// Validation
	if req.Email == "" || req.Password == "" || req.ClientId == "" {
		return &authv1.LoginUserResponse{
			Success: false,
			Message: "Email, password, and client ID are required",
		}, nil
	}

	// Check if client exists
	clientExists, err := s.repo.IsClientExists(req.ClientId)
	if err != nil {
		log.Printf("Error checking client existence: %v", err)
		return &authv1.LoginUserResponse{
			Success: false,
			Message: "Internal server error",
		}, nil
	}
	if !clientExists {
		return &authv1.LoginUserResponse{
			Success: false,
			Message: "Invalid client ID",
		}, nil
	}

	// Get user by email
	user, err := s.repo.GetUserByEmail(req.Email)
	if err != nil {
		log.Printf("Error getting user by email: %v", err)
		return &authv1.LoginUserResponse{
			Success: false,
			Message: "Invalid credentials",
		}, nil
	}

	// Check if user belongs to the client
	if user.ClientId != req.ClientId {
		return &authv1.LoginUserResponse{
			Success: false,
			Message: "Invalid credentials",
		}, nil
	}

	// Verify password
	if !utils.CheckPasswordHash(req.Password, user.Password) {
		return &authv1.LoginUserResponse{
			Success: false,
			Message: "Invalid credentials",
		}, nil
	}

	// Generate refresh token
	refreshToken, err := utils.GenerateRefreshToken()
	if err != nil {
		log.Printf("Error generating refresh token: %v", err)
		return &authv1.LoginUserResponse{
			Success: false,
			Message: "Internal server error",
		}, nil
	}

	// Generate JWT token with refresh token in payload
	accessToken, expiresAt, err := utils.GenerateJWTToken(user.UserId, user.UserName, user.ClientId, refreshToken)
	if err != nil {
		log.Printf("Error generating JWT token: %v", err)
		return &authv1.LoginUserResponse{
			Success: false,
			Message: "Internal server error",
		}, nil
	}

	// Create or update session (only one session per user-client pair)
	session := &models.Session{
		UserId:       user.UserId,
		ClientId:     user.ClientId,
		RefreshToken: refreshToken,
		UserAgent:    req.UserAgent,
		ExpiresAt:    time.Now().Add(7 * 24 * time.Hour), // 7 days
	}

	if err := s.repo.CreateOrUpdateSession(session); err != nil {
		log.Printf("Error creating/updating session: %v", err)
		return &authv1.LoginUserResponse{
			Success: false,
			Message: "Internal server error",
		}, nil
	}

	userProfile := &authv1.UserProfile{
		UserId:    user.UserId,
		Username:  user.UserName,
		Email:     user.EmailId,
		ClientId:  user.ClientId,
		CreatedAt: timestamppb.New(user.CreatedAt),
	}

	log.Printf("User logged in successfully: %s", user.UserId)
	return &authv1.LoginUserResponse{
		Success:      true,
		Message:      "Login successful",
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresAt:    timestamppb.New(expiresAt),
		User:         userProfile,
	}, nil
}

func (s *AuthServiceServerImpl) ValidateToken(ctx context.Context, req *authv1.ValidateTokenRequest) (*authv1.ValidateTokenResponse, error) {
	log.Printf("ValidateToken request received")

	if req.AccessToken == "" {
		return &authv1.ValidateTokenResponse{
			Valid:   false,
			Message: "Access token is required",
		}, nil
	}

	claims, err := utils.ValidateJWTToken(req.AccessToken)
	if err != nil {
		log.Printf("Error validating JWT token: %v", err)
		return &authv1.ValidateTokenResponse{
			Valid:   false,
			Message: "Invalid token",
		}, nil
	}

	// Check if user still exists
	user, err := s.repo.GetUserByID(claims.UserID)
	if err != nil {
		log.Printf("Error getting user by ID: %v", err)
		return &authv1.ValidateTokenResponse{
			Valid:   false,
			Message: "User not found",
		}, nil
	}

	// Validate username matches
	if user.UserName != claims.Username {
		log.Printf("Username mismatch in token claims")
		return &authv1.ValidateTokenResponse{
			Valid:   false,
			Message: "Invalid token claims",
		}, nil
	}

	// Validate client ID matches
	if user.ClientId != claims.ClientID {
		log.Printf("Client ID mismatch in token claims")
		return &authv1.ValidateTokenResponse{
			Valid:   false,
			Message: "Invalid token claims",
		}, nil
	}

	// Validate refresh token exists in database (for additional security)
	session, err := s.repo.GetSessionByUserAndClient(user.UserId, user.ClientId)
	if err != nil || session.RefreshToken != claims.RefreshToken {
		log.Printf("Refresh token validation failed")
		return &authv1.ValidateTokenResponse{
			Valid:   false,
			Message: "Invalid session",
		}, nil
	}

	return &authv1.ValidateTokenResponse{
		Valid:     true,
		Message:   "Token is valid",
		UserId:    user.UserId,
		ExpiresAt: timestamppb.New(claims.ExpiresAt.Time),
	}, nil
}

func (s *AuthServiceServerImpl) RefreshToken(ctx context.Context, req *authv1.RefreshTokenRequest) (*authv1.RefreshTokenResponse, error) {
	log.Printf("RefreshToken request received")

	if req.RefreshToken == "" || req.ClientId == "" {
		return &authv1.RefreshTokenResponse{
			Success: false,
			Message: "Refresh token and client ID are required",
		}, nil
	}

	// Get session by refresh token
	session, err := s.repo.GetSessionByRefreshToken(req.RefreshToken)
	if err != nil {
		log.Printf("Error getting session by refresh token: %v", err)
		return &authv1.RefreshTokenResponse{
			Success: false,
			Message: "Invalid refresh token",
		}, nil
	}

	// Get user
	user, err := s.repo.GetUserByID(session.UserId)
	if err != nil {
		log.Printf("Error getting user by ID: %v", err)
		return &authv1.RefreshTokenResponse{
			Success: false,
			Message: "User not found",
		}, nil
	}

	// Check if user belongs to the client
	if user.ClientId != req.ClientId {
		return &authv1.RefreshTokenResponse{
			Success: false,
			Message: "Invalid client ID",
		}, nil
	}

	// Generate new tokens
	newRefreshToken, err := utils.GenerateRefreshToken()
	if err != nil {
		log.Printf("Error generating refresh token: %v", err)
		return &authv1.RefreshTokenResponse{
			Success: false,
			Message: "Internal server error",
		}, nil
	}

	// Generate JWT token with new refresh token in payload
	accessToken, expiresAt, err := utils.GenerateJWTToken(user.UserId, user.UserName, user.ClientId, newRefreshToken)
	if err != nil {
		log.Printf("Error generating JWT token: %v", err)
		return &authv1.RefreshTokenResponse{
			Success: false,
			Message: "Internal server error",
		}, nil
	}

	// Update session with new refresh token
	session.RefreshToken = newRefreshToken
	session.ExpiresAt = time.Now().Add(7 * 24 * time.Hour) // 7 days
	if err := s.repo.CreateOrUpdateSession(session); err != nil {
		log.Printf("Error updating session: %v", err)
		return &authv1.RefreshTokenResponse{
			Success: false,
			Message: "Internal server error",
		}, nil
	}

	log.Printf("Token refreshed successfully for user: %s", user.UserId)
	return &authv1.RefreshTokenResponse{
		Success:      true,
		Message:      "Token refreshed successfully",
		AccessToken:  accessToken,
		RefreshToken: newRefreshToken,
		ExpiresAt:    timestamppb.New(expiresAt),
	}, nil
}

func (s *AuthServiceServerImpl) LogoutUser(ctx context.Context, req *authv1.LogoutUserRequest) (*authv1.LogoutUserResponse, error) {
	log.Printf("LogoutUser request received")

	if req.RefreshToken == "" {
		return &authv1.LogoutUserResponse{
			Success: false,
			Message: "Refresh token is required",
		}, nil
	}

	// Delete session by refresh token
	if err := s.repo.DeleteSessionByRefreshToken(req.RefreshToken); err != nil {
		log.Printf("Error deleting session: %v", err)
		return &authv1.LogoutUserResponse{
			Success: false,
			Message: "Invalid refresh token",
		}, nil
	}

	log.Printf("User logged out successfully")
	return &authv1.LogoutUserResponse{
		Success: true,
		Message: "Logged out successfully",
	}, nil
}

func (s *AuthServiceServerImpl) RegisterClient(ctx context.Context, req *authv1.RegisterClientRequest) (*authv1.RegisterClientResponse, error) {
	log.Printf("RegisterClient request received for client: %s", req.ClientName)

	if req.ClientName == "" {
		return &authv1.RegisterClientResponse{
			Success: false,
			Message: "Client name is required",
		}, nil
	}

	// Generate client ID and secret
	clientID := utils.GenerateUUID()
	clientSecret, err := utils.GenerateClientSecret()
	if err != nil {
		log.Printf("Error generating client secret: %v", err)
		return &authv1.RegisterClientResponse{
			Success: false,
			Message: "Internal server error",
		}, nil
	}

	// Create client
	client := &models.Client{
		ClientId:     clientID,
		ClientName:   req.ClientName,
		ClientSecret: clientSecret,
	}

	if err := s.repo.CreateClient(client); err != nil {
		log.Printf("Error creating client: %v", err)
		return &authv1.RegisterClientResponse{
			Success: false,
			Message: "Failed to create client",
		}, nil
	}

	log.Printf("Client registered successfully: %s", clientID)
	return &authv1.RegisterClientResponse{
		Success:      true,
		Message:      "Client registered successfully",
		ClientId:     clientID,
		ClientSecret: clientSecret,
	}, nil
}

func (s *AuthServiceServerImpl) GetUserProfile(ctx context.Context, req *authv1.GetUserProfileRequest) (*authv1.GetUserProfileResponse, error) {
	log.Printf("GetUserProfile request received")

	if req.AccessToken == "" {
		return &authv1.GetUserProfileResponse{
			Success: false,
			Message: "Access token is required",
		}, nil
	}

	claims, err := utils.ValidateJWTToken(req.AccessToken)
	if err != nil {
		log.Printf("Error validating JWT token: %v", err)
		return &authv1.GetUserProfileResponse{
			Success: false,
			Message: "Invalid token",
		}, nil
	}

	user, err := s.repo.GetUserByID(claims.UserID)
	if err != nil {
		log.Printf("Error getting user by ID: %v", err)
		return &authv1.GetUserProfileResponse{
			Success: false,
			Message: "User not found",
		}, nil
	}

	userProfile := &authv1.UserProfile{
		UserId:    user.UserId,
		Username:  user.UserName,
		Email:     user.EmailId,
		ClientId:  user.ClientId,
		CreatedAt: timestamppb.New(user.CreatedAt),
	}

	return &authv1.GetUserProfileResponse{
		Success: true,
		Message: "User profile retrieved successfully",
		User:    userProfile,
	}, nil
}

func (s *AuthServiceServerImpl) ChangePassword(ctx context.Context, req *authv1.ChangePasswordRequest) (*authv1.ChangePasswordResponse, error) {
	log.Printf("ChangePassword request received")

	if req.AccessToken == "" || req.CurrentPassword == "" || req.NewPassword == "" {
		return &authv1.ChangePasswordResponse{
			Success: false,
			Message: "Access token, current password, and new password are required",
		}, nil
	}

	// Validate access token
	claims, err := utils.ValidateJWTToken(req.AccessToken)
	if err != nil {
		log.Printf("Error validating JWT token: %v", err)
		return &authv1.ChangePasswordResponse{
			Success: false,
			Message: "Invalid access token",
		}, nil
	}

	// Get user
	user, err := s.repo.GetUserByID(claims.UserID)
	if err != nil {
		log.Printf("Error getting user by ID: %v", err)
		return &authv1.ChangePasswordResponse{
			Success: false,
			Message: "User not found",
		}, nil
	}

	// Verify current password
	if !utils.CheckPasswordHash(req.CurrentPassword, user.Password) {
		return &authv1.ChangePasswordResponse{
			Success: false,
			Message: "Current password is incorrect",
		}, nil
	}

	// Hash new password
	hashedNewPassword, err := utils.HashPassword(req.NewPassword)
	if err != nil {
		log.Printf("Error hashing new password: %v", err)
		return &authv1.ChangePasswordResponse{
			Success: false,
			Message: "Internal server error",
		}, nil
	}

	// Update password
	user.Password = hashedNewPassword
	if err := s.repo.UpdateUser(user); err != nil {
		log.Printf("Error updating user password: %v", err)
		return &authv1.ChangePasswordResponse{
			Success: false,
			Message: "Internal server error",
		}, nil
	}

	// Invalidate all sessions for this user (security requirement)
	if err := s.repo.DeleteAllUserSessions(user.UserId); err != nil {
		log.Printf("Error invalidating user sessions: %v", err)
		return &authv1.ChangePasswordResponse{
			Success: false,
			Message: "Password changed but failed to invalidate sessions",
		}, nil
	}

	log.Printf("Password changed successfully for user: %s", user.UserId)
	return &authv1.ChangePasswordResponse{
		Success: true,
		Message: "Password changed successfully. Please log in again.",
	}, nil
}

// Helper functions
func (s *AuthServiceServerImpl) validateUserRegistration(req *authv1.RegisterUserRequest) error {
	if req.Username == "" {
		return fmt.Errorf("username is required")
	}

	if req.Email == "" {
		return fmt.Errorf("email is required")
	}

	if !s.isValidEmail(req.Email) {
		return fmt.Errorf("invalid email format")
	}

	if req.Password == "" {
		return fmt.Errorf("password is required")
	}

	if len(req.Password) < 8 {
		return fmt.Errorf("password must be at least 8 characters long")
	}

	if req.ClientId == "" {
		return fmt.Errorf("client ID is required")
	}

	return nil
}

func (s *AuthServiceServerImpl) isValidEmail(email string) bool {
	email = strings.TrimSpace(email)
	emailRegex := regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)
	return emailRegex.MatchString(email)
}
