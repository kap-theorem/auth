package service

import (
	"authservice/pkg/models"
	"authservice/pkg/ratelimit"
	"authservice/pkg/repository"
	"authservice/pkg/utils"
	authv1 "authservice/proto/auth/v1"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"

	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

type AuthServiceServerImpl struct {
	authv1.UnimplementedAuthServiceServer
	repo *repository.AuthRepository
}

func userToProfile(u *models.User) *authv1.UserProfile {
	return &authv1.UserProfile{
		UserId:       u.UserID,
		Username:     u.UserName,
		Email:        u.Email,
		ClientId:     u.ClientID,
		CreatedAt:    timestamppb.New(u.CreatedAt),
		LockUsername: u.LockUsername,
		LockEmail:    u.LockEmail,
		LockPassword: u.LockPassword,
	}
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
	clientExists, err := s.repo.IsClientExists(ctx, req.ClientId)
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

	// Check if client is invite-only and validate invite token
	clientConfig, err := s.repo.GetClientConfig(ctx, req.ClientId)
	if err != nil {
		log.Printf("Error getting client config: %v", err)
		return &authv1.RegisterUserResponse{
			Success: false,
			Message: "Internal server error",
		}, nil
	}
	if clientConfig.InviteOnly {
		if req.InviteToken == "" {
			return &authv1.RegisterUserResponse{
				Success: false,
				Message: "This application requires an invite token to register",
			}, nil
		}
		inviteToken, err := s.repo.GetInviteToken(ctx, req.InviteToken)
		if err != nil {
			return &authv1.RegisterUserResponse{
				Success: false,
				Message: "Invalid or expired invite token",
			}, nil
		}
		if inviteToken.ClientID != req.ClientId {
			return &authv1.RegisterUserResponse{
				Success: false,
				Message: "Invite token is not valid for this application",
			}, nil
		}
		// If invite token has a pre-filled email, verify it matches
		if inviteToken.Email != "" && inviteToken.Email != req.Email {
			return &authv1.RegisterUserResponse{
				Success: false,
				Message: "Email does not match the invite",
			}, nil
		}
		// Mark token as used after all validation passes (we'll do this after user creation)
		defer func() {
			_ = s.repo.MarkInviteTokenUsed(ctx, req.InviteToken)
		}()
	}

	// Check if email already exists
	emailExists, err := s.repo.IsEmailExists(ctx, req.Email)
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
		UserID:   userID,
		UserName: req.Username,
		Email:    req.Email,
		Password: hashedPassword,
		ClientID: req.ClientId,
	}

	if err := s.repo.CreateUser(ctx, user); err != nil {
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

func (s *AuthServiceServerImpl) GetToken(ctx context.Context, req *authv1.GetTokenRequest) (*authv1.GetTokenResponse, error) {
	log.Printf("GetToken request received for identifier: %s", req.LoginIdentifier)

	// Validation
	if req.LoginIdentifier == "" || req.Password == "" || req.ClientId == "" {
		return &authv1.GetTokenResponse{
			Success: false,
			Message: "Login identifier, password, and client ID are required",
		}, nil
	}

	// Rate limiting check
	if limited, remaining := ratelimit.IsRateLimited(req.LoginIdentifier); limited {
		log.Printf("Rate limited login attempt for identifier: %s", req.LoginIdentifier)
		return &authv1.GetTokenResponse{
			Success: false,
			Message: ratelimit.FormatLockoutMessage(remaining),
		}, nil
	}

	// Check if client exists
	clientExists, err := s.repo.IsClientExists(ctx, req.ClientId)
	if err != nil {
		log.Printf("Error checking client existence: %v", err)
		return &authv1.GetTokenResponse{
			Success: false,
			Message: "Internal server error",
		}, nil
	}
	if !clientExists {
		return &authv1.GetTokenResponse{
			Success: false,
			Message: "Invalid client ID",
		}, nil
	}

	// Get user by identifier (email or username)
	user, err := s.repo.GetUserByIdentifier(ctx, req.LoginIdentifier)
	if err != nil {
		log.Printf("Error getting user by identifier: %v", err)
		ratelimit.RecordFailedAttempt(req.LoginIdentifier)
		return &authv1.GetTokenResponse{
			Success: false,
			Message: "Invalid credentials",
		}, nil
	}

	// Check if user belongs to the client
	if user.ClientID != req.ClientId {
		ratelimit.RecordFailedAttempt(req.LoginIdentifier)
		return &authv1.GetTokenResponse{
			Success: false,
			Message: "Invalid credentials",
		}, nil
	}

	// Verify password
	if !utils.CheckPasswordHash(req.Password, user.Password) {
		ratelimit.RecordFailedAttempt(req.LoginIdentifier)
		return &authv1.GetTokenResponse{
			Success: false,
			Message: "Invalid credentials",
		}, nil
	}

	// Generate refresh token
	refreshToken, err := utils.GenerateRefreshToken()
	if err != nil {
		log.Printf("Error generating refresh token: %v", err)
		return &authv1.GetTokenResponse{
			Success: false,
			Message: "Internal server error",
		}, nil
	}

	// Generate JWT token with refresh token in payload
	accessToken, expiresAt, err := utils.GenerateJWTToken(user.UserID, user.UserName, user.ClientID, refreshToken)
	if err != nil {
		log.Printf("Error generating JWT token: %v", err)
		return &authv1.GetTokenResponse{
			Success: false,
			Message: "Internal server error",
		}, nil
	}

	// Create or update session (only one session per user-client pair)
	session := &models.Session{
		UserID:       user.UserID,
		ClientID:     user.ClientID,
		RefreshToken: refreshToken,
		UserAgent:    req.UserAgent,
		ExpiresAt:    time.Now().Add(7 * 24 * time.Hour), // 7 days
	}

	if err := s.repo.CreateOrUpdateSession(ctx, session); err != nil {
		log.Printf("Error creating/updating session: %v", err)
		return &authv1.GetTokenResponse{
			Success: false,
			Message: "Internal server error",
		}, nil
	}

	userProfile := userToProfile(user)

	// Clear rate limit on successful login
	ratelimit.ResetAttempts(req.LoginIdentifier)

	log.Printf("User logged in successfully: %s", user.UserID)
	return &authv1.GetTokenResponse{
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
	user, err := s.repo.GetUserByID(ctx, claims.UserID)
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
	if user.ClientID != claims.ClientID {
		log.Printf("Client ID mismatch in token claims")
		return &authv1.ValidateTokenResponse{
			Valid:   false,
			Message: "Invalid token claims",
		}, nil
	}

	// Validate refresh token exists in database (for additional security)
	session, err := s.repo.GetSessionByUserAndClient(ctx, user.UserID, user.ClientID)
	if err != nil || session.RefreshToken != claims.RefreshToken {
		log.Printf("Refresh token validation failed")
		return &authv1.ValidateTokenResponse{
			Valid:   false,
			Message: "Invalid session",
		}, nil
	}

	userProfile := userToProfile(user)

	return &authv1.ValidateTokenResponse{
		Valid:     true,
		Message:   "Token is valid",
		UserId:    user.UserID,
		ExpiresAt: timestamppb.New(claims.ExpiresAt.Time),
		User:      userProfile,
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
	session, err := s.repo.GetSessionByRefreshToken(ctx, req.RefreshToken)
	if err != nil {
		log.Printf("Error getting session by refresh token: %v", err)
		return &authv1.RefreshTokenResponse{
			Success: false,
			Message: "Invalid refresh token",
		}, nil
	}

	// Get user
	user, err := s.repo.GetUserByID(ctx, session.UserID)
	if err != nil {
		log.Printf("Error getting user by ID: %v", err)
		return &authv1.RefreshTokenResponse{
			Success: false,
			Message: "User not found",
		}, nil
	}

	// Check if user belongs to the client
	if user.ClientID != req.ClientId {
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
	accessToken, expiresAt, err := utils.GenerateJWTToken(user.UserID, user.UserName, user.ClientID, newRefreshToken)
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
	if err := s.repo.CreateOrUpdateSession(ctx, session); err != nil {
		log.Printf("Error updating session: %v", err)
		return &authv1.RefreshTokenResponse{
			Success: false,
			Message: "Internal server error",
		}, nil
	}

	log.Printf("Token refreshed successfully for user: %s", user.UserID)
	return &authv1.RefreshTokenResponse{
		Success:      true,
		Message:      "Token refreshed successfully",
		AccessToken:  accessToken,
		RefreshToken: newRefreshToken,
		ExpiresAt:    timestamppb.New(expiresAt),
	}, nil
}

func (s *AuthServiceServerImpl) RevokeToken(ctx context.Context, req *authv1.RevokeTokenRequest) (*authv1.RevokeTokenResponse, error) {
	log.Printf("RevokeToken request received")

	if req.RefreshToken == "" {
		return &authv1.RevokeTokenResponse{
			Success: false,
			Message: "Refresh token is required",
		}, nil
	}

	// Delete session by refresh token
	if err := s.repo.DeleteSessionByRefreshToken(ctx, req.RefreshToken); err != nil {
		log.Printf("Error deleting session: %v", err)
		return &authv1.RevokeTokenResponse{
			Success: false,
			Message: "Invalid refresh token",
		}, nil
	}

	log.Printf("Token revoked successfully")
	return &authv1.RevokeTokenResponse{
		Success: true,
		Message: "Token revoked successfully",
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
		ClientID:     clientID,
		ClientName:   req.ClientName,
		ClientSecret: clientSecret,
	}

	if err := s.repo.CreateClient(ctx, client); err != nil {
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

func (s *AuthServiceServerImpl) ValidateClientCredentials(ctx context.Context, req *authv1.ValidateClientCredentialsRequest) (*authv1.ValidateClientCredentialsResponse, error) {
	log.Printf("ValidateClientCredentials request received for client: %s", req.ClientId)

	if req.ClientId == "" || req.ClientSecret == "" {
		return &authv1.ValidateClientCredentialsResponse{
			Valid:   false,
			Message: "client_id and client_secret are required",
		}, nil
	}

	client, err := s.repo.ValidateClient(ctx, req.ClientId, req.ClientSecret)
	if err != nil {
		log.Printf("Invalid client credentials for ID: %s", req.ClientId)
		return &authv1.ValidateClientCredentialsResponse{
			Valid:   false,
			Message: "Invalid client credentials",
		}, nil
	}

	log.Printf("Client credentials verified for: %s", client.ClientName)
	return &authv1.ValidateClientCredentialsResponse{
		Valid:      true,
		Message:    "Credentials valid",
		ClientName: client.ClientName,
	}, nil
}

func (s *AuthServiceServerImpl) ChangeUserPassword(ctx context.Context, req *authv1.ChangeUserPasswordRequest) (*authv1.ChangeUserPasswordResponse, error) {
	log.Printf("ChangePassword request received")

	if req.AccessToken == "" || req.CurrentPassword == "" || req.NewPassword == "" {
		return &authv1.ChangeUserPasswordResponse{
			Success: false,
			Message: "Access token, current password, and new password are required",
		}, nil
	}

	// Validate access token
	claims, err := utils.ValidateJWTToken(req.AccessToken)
	if err != nil {
		log.Printf("Error validating JWT token: %v", err)
		return &authv1.ChangeUserPasswordResponse{
			Success: false,
			Message: "Invalid access token",
		}, nil
	}

	// Get user
	user, err := s.repo.GetUserByID(ctx, claims.UserID)
	if err != nil {
		log.Printf("Error getting user by ID: %v", err)
		return &authv1.ChangeUserPasswordResponse{
			Success: false,
			Message: "User not found",
		}, nil
	}

	// Check if password is locked by admin
	if user.LockPassword {
		return &authv1.ChangeUserPasswordResponse{
			Success: false,
			Message: "Password changes are disabled by your administrator",
		}, nil
	}

	// Verify current password
	if !utils.CheckPasswordHash(req.CurrentPassword, user.Password) {
		return &authv1.ChangeUserPasswordResponse{
			Success: false,
			Message: "Current password is incorrect",
		}, nil
	}

	// Hash new password
	hashedNewPassword, err := utils.HashPassword(req.NewPassword)
	if err != nil {
		log.Printf("Error hashing new password: %v", err)
		return &authv1.ChangeUserPasswordResponse{
			Success: false,
			Message: "Internal server error",
		}, nil
	}

	// Update password
	user.Password = hashedNewPassword
	if err := s.repo.UpdateUser(ctx, user); err != nil {
		log.Printf("Error updating user password: %v", err)
		return &authv1.ChangeUserPasswordResponse{
			Success: false,
			Message: "Internal server error",
		}, nil
	}

	// Invalidate all sessions for this user (security requirement)
	if err := s.repo.DeleteAllUserSessions(ctx, user.UserID); err != nil {
		log.Printf("Error invalidating user sessions: %v", err)
		return &authv1.ChangeUserPasswordResponse{
			Success: false,
			Message: "Password changed but failed to invalidate sessions",
		}, nil
	}

	log.Printf("Password changed successfully for user: %s", user.UserID)
	return &authv1.ChangeUserPasswordResponse{
		Success: true,
		Message: "Password changed successfully. Please log in again.",
	}, nil
}

func (s *AuthServiceServerImpl) UpdateUserProfile(ctx context.Context, req *authv1.UpdateUserProfileRequest) (*authv1.UpdateUserProfileResponse, error) {
	log.Printf("UpdateUserProfile request received")

	if req.AccessToken == "" {
		return &authv1.UpdateUserProfileResponse{
			Success: false,
			Message: "Access token is required",
		}, nil
	}

	// Validate access token
	claims, err := utils.ValidateJWTToken(req.AccessToken)
	if err != nil {
		log.Printf("Error validating JWT token: %v", err)
		return &authv1.UpdateUserProfileResponse{
			Success: false,
			Message: "Invalid access token",
		}, nil
	}

	// Get user
	user, err := s.repo.GetUserByID(ctx, claims.UserID)
	if err != nil {
		log.Printf("Error getting user by ID: %v", err)
		return &authv1.UpdateUserProfileResponse{
			Success: false,
			Message: "User not found",
		}, nil
	}

	// Check field locks set by admin
	if req.NewEmail != "" && req.NewEmail != user.Email && user.LockEmail {
		return &authv1.UpdateUserProfileResponse{
			Success: false,
			Message: "Email changes are disabled by your administrator",
		}, nil
	}
	if req.NewUsername != "" && req.NewUsername != user.UserName && user.LockUsername {
		return &authv1.UpdateUserProfileResponse{
			Success: false,
			Message: "Username changes are disabled by your administrator",
		}, nil
	}

	// Validate new email if provided
	if req.NewEmail != "" && req.NewEmail != user.Email {
		if !s.isValidEmail(req.NewEmail) {
			return &authv1.UpdateUserProfileResponse{
				Success: false,
				Message: "Invalid email format",
			}, nil
		}
		
		emailExists, err := s.repo.IsEmailExists(ctx, req.NewEmail)
		if err != nil {
			log.Printf("Error checking email existence: %v", err)
			return &authv1.UpdateUserProfileResponse{
				Success: false,
				Message: "Internal server error",
			}, nil
		}
		if emailExists {
			return &authv1.UpdateUserProfileResponse{
				Success: false,
				Message: "Email already registered",
			}, nil
		}
		user.Email = req.NewEmail
	}

	// Validate new username if provided
	if req.NewUsername != "" && req.NewUsername != user.UserName {
		// Basic username validation (e.g., length)
		if len(req.NewUsername) < 3 {
			return &authv1.UpdateUserProfileResponse{
				Success: false,
				Message: "Username must be at least 3 characters long",
			}, nil
		}

		usernameExists, err := s.repo.IsUsernameExists(ctx, req.NewUsername)
		if err != nil {
			log.Printf("Error checking username existence: %v", err)
			return &authv1.UpdateUserProfileResponse{
				Success: false,
				Message: "Internal server error",
			}, nil
		}
		if usernameExists {
			return &authv1.UpdateUserProfileResponse{
				Success: false,
				Message: "Username already taken",
			}, nil
		}
		user.UserName = req.NewUsername
	}

	// Update user in DB
	if err := s.repo.UpdateUser(ctx, user); err != nil {
		log.Printf("Error updating user profile: %v", err)
		return &authv1.UpdateUserProfileResponse{
			Success: false,
			Message: "Failed to update profile",
		}, nil
	}

	log.Printf("User profile updated successfully: %s", user.UserID)

	return &authv1.UpdateUserProfileResponse{
		Success: true,
		Message: "Profile updated successfully",
		User:    userToProfile(user),
	}, nil
}

func (s *AuthServiceServerImpl) ChangeClientSecret(ctx context.Context, req *authv1.ChangeClientSecretRequest) (*authv1.ChangeClientSecretResponse, error) {
	log.Printf("ChangeClientSecret request received for client: %s", req.ClientId)

	if req.ClientId == "" || req.CurrentSecret == "" {
		return &authv1.ChangeClientSecretResponse{Success: false, Message: "client_id and current_secret are required"}, nil
	}

	// Validate current secret
	if _, err := s.repo.ValidateClient(ctx, req.ClientId, req.CurrentSecret); err != nil {
		return &authv1.ChangeClientSecretResponse{Success: false, Message: "Invalid client credentials"}, nil
	}

	// Decide new secret
	newSecret := req.NewSecret
	if strings.TrimSpace(newSecret) == "" {
		generated, err := utils.GenerateClientSecret()
		if err != nil {
			log.Printf("Error generating client secret: %v", err)
			return &authv1.ChangeClientSecretResponse{Success: false, Message: "Internal server error"}, nil
		}
		newSecret = generated
	}

	if err := s.repo.UpdateClientSecret(ctx, req.ClientId, newSecret); err != nil {
		log.Printf("Error updating client secret: %v", err)
		return &authv1.ChangeClientSecretResponse{Success: false, Message: "Failed to update client secret"}, nil
	}

	return &authv1.ChangeClientSecretResponse{
		Success:      true,
		Message:      "Client secret updated successfully",
		ClientId:     req.ClientId,
		ClientSecret: newSecret,
	}, nil
}

func (s *AuthServiceServerImpl) ListClients(ctx context.Context, _ *emptypb.Empty) (*authv1.ListClientsResponse, error) {
	log.Printf("ListClients request received")

	clients, err := s.repo.GetAllClients(ctx)
	if err != nil {
		log.Printf("Error fetching clients: %v", err)
		return nil, fmt.Errorf("failed to fetch clients: %v", err)
	}

	resClients := make([]*authv1.ClientInfo, len(clients))
	for i, c := range clients {
		resClients[i] = &authv1.ClientInfo{
			ClientId:   c.ClientID,
			ClientName: c.ClientName,
			CreatedAt:  timestamppb.New(c.CreatedAt),
		}
	}

	return &authv1.ListClientsResponse{
		Clients: resClients,
	}, nil
}

func (s *AuthServiceServerImpl) GetSystemStats(ctx context.Context, _ *emptypb.Empty) (*authv1.GetSystemStatsResponse, error) {
	log.Printf("GetSystemStats request received")

	totalUsers, totalClients, activeSessions, err := s.repo.GetSystemStats(ctx)
	if err != nil {
		log.Printf("Error fetching system stats: %v", err)
		return nil, fmt.Errorf("failed to fetch system stats: %v", err)
	}

	return &authv1.GetSystemStatsResponse{
		TotalUsers:     totalUsers,
		TotalClients:   totalClients,
		ActiveSessions: activeSessions,
		SystemHealth:   1.0, // Hardcoded for now, could be dynamic
	}, nil
}

func (s *AuthServiceServerImpl) ListClientUsers(ctx context.Context, req *authv1.ListClientUsersRequest) (*authv1.ListClientUsersResponse, error) {
	log.Printf("ListClientUsers request received for client: %s", req.ClientId)

	users, err := s.repo.GetUsersByClientID(ctx, req.ClientId)
	if err != nil {
		log.Printf("Error fetching client users: %v", err)
		return nil, fmt.Errorf("failed to fetch client users: %v", err)
	}

	resUsers := make([]*authv1.UserProfile, len(users))
	for i, u := range users {
		resUsers[i] = userToProfile(&u)
	}

	return &authv1.ListClientUsersResponse{
		Users: resUsers,
	}, nil
}

func (s *AuthServiceServerImpl) GetClientStats(ctx context.Context, req *authv1.GetClientStatsRequest) (*authv1.GetClientStatsResponse, error) {
	log.Printf("GetClientStats request received for client: %s", req.ClientId)

	totalUsers, last24hLogins, err := s.repo.GetClientStats(ctx, req.ClientId)
	if err != nil {
		log.Printf("Error fetching client stats: %v", err)
		return nil, fmt.Errorf("failed to fetch client stats: %v", err)
	}

	newUsersLast7d, err := s.repo.CountNewUsersByClient(ctx, req.ClientId, 7)
	if err != nil {
		log.Printf("Error fetching new users count: %v", err)
		return nil, fmt.Errorf("failed to fetch new users count: %v", err)
	}

	activeSessions, err := s.repo.CountActiveSessionsByClient(ctx, req.ClientId)
	if err != nil {
		log.Printf("Error fetching active sessions count: %v", err)
		return nil, fmt.Errorf("failed to fetch active sessions count: %v", err)
	}

	return &authv1.GetClientStatsResponse{
		TotalUsers:      totalUsers,
		Last_24HLogins:  last24hLogins,
		NewUsersLast_7D: newUsersLast7d,
		ActiveSessions:  activeSessions,
	}, nil
}

func (s *AuthServiceServerImpl) CreateClientUser(ctx context.Context, req *authv1.CreateClientUserRequest) (*authv1.CreateClientUserResponse, error) {
	log.Printf("CreateClientUser request received for username: %s", req.Username)

	// Admin-level checks would typically happen at the API Gateway or interceptor level
	// assuming they are allowed here based on the clientID.

	if req.Username == "" || req.Password == "" || req.ClientId == "" {
		return &authv1.CreateClientUserResponse{Success: false, Message: "Username, password and client ID are required"}, nil
	}

	exists, err := s.repo.IsUsernameExists(ctx, req.Username)
	if err != nil || exists {
		return &authv1.CreateClientUserResponse{Success: false, Message: "Username already exists"}, nil
	}

	if req.Email != "" && !s.isValidEmail(req.Email) {
		return &authv1.CreateClientUserResponse{Success: false, Message: "Invalid email format"}, nil
	}

	if req.Email != "" {
		exists, err = s.repo.IsEmailExists(ctx, req.Email)
		if err != nil || exists {
			return &authv1.CreateClientUserResponse{Success: false, Message: "Email already exists"}, nil
		}
	}

	hashedPassword, err := utils.HashPassword(req.Password)
	if err != nil {
		return &authv1.CreateClientUserResponse{Success: false, Message: "Internal server error hashing password"}, nil
	}

	userID := utils.GenerateUUID()
	user := &models.User{
		UserID:   userID,
		UserName: req.Username,
		Email:    req.Email,
		Password: hashedPassword,
		ClientID: req.ClientId,
	}

	if err := s.repo.CreateUser(ctx, user); err != nil {
		log.Printf("Error creating user: %v", err)
		return &authv1.CreateClientUserResponse{Success: false, Message: "Failed to create user"}, nil
	}

	return &authv1.CreateClientUserResponse{
		Success: true,
		Message: "User created successfully",
		User:    userToProfile(user),
	}, nil
}

func (s *AuthServiceServerImpl) DeleteClientUser(ctx context.Context, req *authv1.DeleteClientUserRequest) (*emptypb.Empty, error) {
	log.Printf("DeleteClientUser request received for user: %s under client: %s", req.UserId, req.ClientId)

	// Validate user belongs to the client before deleting
	user, err := s.repo.GetUserByID(ctx, req.UserId)
	if err != nil || user.ClientID != req.ClientId {
		log.Printf("User not found or client ID mismatch")
		return &emptypb.Empty{}, nil
	}

	// Delete associated sessions first
	_ = s.repo.DeleteAllUserSessions(ctx, req.UserId)

	if err := s.repo.DeleteUser(ctx, req.UserId); err != nil {
		log.Printf("Error deleting user: %v", err)
	}

	return &emptypb.Empty{}, nil
}

func (s *AuthServiceServerImpl) UpdateClientUser(ctx context.Context, req *authv1.UpdateClientUserRequest) (*authv1.UpdateClientUserResponse, error) {
	log.Printf("UpdateClientUser request for user: %s under client: %s", req.UserId, req.ClientId)

	user, err := s.repo.GetUserByID(ctx, req.UserId)
	if err != nil || user.ClientID != req.ClientId {
		return &authv1.UpdateClientUserResponse{Success: false, Message: "User not found or access denied"}, nil
	}

	if req.NewUsername != "" && req.NewUsername != user.UserName {
		user.UserName = req.NewUsername
	}
	if req.NewEmail != "" && req.NewEmail != user.Email {
		user.Email = req.NewEmail
	}
	if req.NewPassword != "" {
		hashed, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
		if err != nil {
			return &authv1.UpdateClientUserResponse{Success: false, Message: "Failed to hash password"}, nil
		}
		user.Password = string(hashed)
	}

	// Update field locks (admin can always set these)
	user.LockUsername = req.LockUsername
	user.LockEmail = req.LockEmail
	user.LockPassword = req.LockPassword

	if err := s.repo.UpdateUser(ctx, user); err != nil {
		log.Printf("Error updating user: %v", err)
		return &authv1.UpdateClientUserResponse{Success: false, Message: "Failed to update user"}, nil
	}

	return &authv1.UpdateClientUserResponse{Success: true, Message: "User updated successfully"}, nil
}

func (s *AuthServiceServerImpl) CreateInviteToken(ctx context.Context, req *authv1.CreateInviteTokenRequest) (*authv1.CreateInviteTokenResponse, error) {
	log.Printf("CreateInviteToken request received for client: %s", req.ClientId)

	if req.ClientId == "" {
		return &authv1.CreateInviteTokenResponse{
			Success: false,
			Message: "Client ID is required",
		}, nil
	}

	// Verify client exists
	exists, err := s.repo.IsClientExists(ctx, req.ClientId)
	if err != nil || !exists {
		return &authv1.CreateInviteTokenResponse{
			Success: false,
			Message: "Invalid client ID",
		}, nil
	}

	// Generate token
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		log.Printf("Error generating invite token: %v", err)
		return &authv1.CreateInviteTokenResponse{
			Success: false,
			Message: "Internal server error",
		}, nil
	}
	tokenStr := hex.EncodeToString(tokenBytes)

	inviteToken := &models.InviteToken{
		Token:     tokenStr,
		ClientID:  req.ClientId,
		Email:     req.Email,
		ExpiresAt: time.Now().Add(7 * 24 * time.Hour), // 7 days
	}

	if err := s.repo.CreateInviteToken(ctx, inviteToken); err != nil {
		log.Printf("Error creating invite token: %v", err)
		return &authv1.CreateInviteTokenResponse{
			Success: false,
			Message: "Failed to create invite token",
		}, nil
	}

	log.Printf("Invite token created for client: %s", req.ClientId)
	return &authv1.CreateInviteTokenResponse{
		Success:     true,
		Message:     "Invite token created successfully. Valid for 7 days.",
		InviteToken: tokenStr,
	}, nil
}

func (s *AuthServiceServerImpl) GetClientConfig(ctx context.Context, req *authv1.GetClientConfigRequest) (*authv1.GetClientConfigResponse, error) {
	client, err := s.repo.GetClientConfig(ctx, req.ClientId)
	if err != nil {
		return &authv1.GetClientConfigResponse{Success: false, Message: "Failed to get config"}, nil
	}

	return &authv1.GetClientConfigResponse{
		Success:    true,
		ClientName: client.ClientName,
		Config: &authv1.ClientConfig{
			DemoMode:   client.DemoMode,
			InviteOnly: client.InviteOnly,
			LoginType:  client.LoginType,
		},
	}, nil
}

func (s *AuthServiceServerImpl) UpdateClientConfig(ctx context.Context, req *authv1.UpdateClientConfigRequest) (*authv1.UpdateClientConfigResponse, error) {
	if req.Config == nil {
		return &authv1.UpdateClientConfigResponse{Success: false, Message: "Config missing"}, nil
	}

	if req.Config.LoginType != "both" && req.Config.LoginType != "email" && req.Config.LoginType != "username" {
		req.Config.LoginType = "both" // Fallback to safe default
	}

	err := s.repo.UpdateClientConfig(ctx, req.ClientId, req.Config.DemoMode, req.Config.InviteOnly, req.Config.LoginType)
	if err != nil {
		return &authv1.UpdateClientConfigResponse{Success: false, Message: "Failed to update config"}, nil
	}

	return &authv1.UpdateClientConfigResponse{
		Success: true,
		Message: "Configuration updated",
		Config:  req.Config,
	}, nil
}

func (s *AuthServiceServerImpl) RequestPasswordReset(ctx context.Context, req *authv1.RequestPasswordResetRequest) (*authv1.RequestPasswordResetResponse, error) {
	log.Printf("RequestPasswordReset request received for email: %s", req.Email)

	if req.Email == "" || req.ClientId == "" {
		return &authv1.RequestPasswordResetResponse{
			Success: false,
			Message: "Email and client ID are required",
		}, nil
	}

	// Check if user exists with this email under this client
	user, err := s.repo.GetUserByEmail(ctx, req.Email)
	if err != nil || user.ClientID != req.ClientId {
		// Don't reveal whether email exists (security best practice)
		return &authv1.RequestPasswordResetResponse{
			Success: true,
			Message: "If this email is registered, a reset token has been generated",
		}, nil
	}

	// Generate reset token
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		log.Printf("Error generating reset token: %v", err)
		return &authv1.RequestPasswordResetResponse{
			Success: false,
			Message: "Internal server error",
		}, nil
	}
	tokenStr := hex.EncodeToString(tokenBytes)

	resetToken := &models.PasswordResetToken{
		Token:     tokenStr,
		UserID:    user.UserID,
		ClientID:  user.ClientID,
		ExpiresAt: time.Now().Add(1 * time.Hour), // 1 hour expiry
	}

	if err := s.repo.CreatePasswordResetToken(ctx, resetToken); err != nil {
		log.Printf("Error creating reset token: %v", err)
		return &authv1.RequestPasswordResetResponse{
			Success: false,
			Message: "Internal server error",
		}, nil
	}

	log.Printf("Password reset token generated for user: %s", user.UserID)
	// NOTE: In production, send this via email instead of returning it
	return &authv1.RequestPasswordResetResponse{
		Success:    true,
		Message:    "Password reset token generated. It expires in 1 hour.",
		ResetToken: tokenStr,
	}, nil
}

func (s *AuthServiceServerImpl) ResetPassword(ctx context.Context, req *authv1.ResetPasswordRequest) (*authv1.ResetPasswordResponse, error) {
	log.Printf("ResetPassword request received")

	if req.ResetToken == "" || req.NewPassword == "" {
		return &authv1.ResetPasswordResponse{
			Success: false,
			Message: "Reset token and new password are required",
		}, nil
	}

	if len(req.NewPassword) < 8 {
		return &authv1.ResetPasswordResponse{
			Success: false,
			Message: "Password must be at least 8 characters long",
		}, nil
	}

	// Validate reset token
	resetToken, err := s.repo.GetPasswordResetToken(ctx, req.ResetToken)
	if err != nil {
		return &authv1.ResetPasswordResponse{
			Success: false,
			Message: "Invalid or expired reset token",
		}, nil
	}

	// Get user
	user, err := s.repo.GetUserByID(ctx, resetToken.UserID)
	if err != nil {
		return &authv1.ResetPasswordResponse{
			Success: false,
			Message: "User not found",
		}, nil
	}

	// Hash new password
	hashedPassword, err := utils.HashPassword(req.NewPassword)
	if err != nil {
		log.Printf("Error hashing password: %v", err)
		return &authv1.ResetPasswordResponse{
			Success: false,
			Message: "Internal server error",
		}, nil
	}

	// Update password
	user.Password = hashedPassword
	if err := s.repo.UpdateUser(ctx, user); err != nil {
		log.Printf("Error updating user password: %v", err)
		return &authv1.ResetPasswordResponse{
			Success: false,
			Message: "Internal server error",
		}, nil
	}

	// Mark token as used
	_ = s.repo.MarkPasswordResetTokenUsed(ctx, req.ResetToken)

	// Invalidate all sessions
	_ = s.repo.DeleteAllUserSessions(ctx, user.UserID)

	log.Printf("Password reset successfully for user: %s", user.UserID)
	return &authv1.ResetPasswordResponse{
		Success: true,
		Message: "Password reset successfully. Please log in with your new password.",
	}, nil
}

func (s *AuthServiceServerImpl) RevokeAllSessions(ctx context.Context, req *authv1.RevokeAllSessionsRequest) (*authv1.RevokeAllSessionsResponse, error) {
	log.Printf("RevokeAllSessions request received")

	if req.AccessToken == "" {
		return &authv1.RevokeAllSessionsResponse{
			Success: false,
			Message: "Access token is required",
		}, nil
	}

	claims, err := utils.ValidateJWTToken(req.AccessToken)
	if err != nil {
		return &authv1.RevokeAllSessionsResponse{
			Success: false,
			Message: "Invalid access token",
		}, nil
	}

	// Delete all sessions except the current one (identified by refresh token in JWT)
	revokedCount, err := s.repo.DeleteOtherUserSessions(ctx, claims.UserID, claims.RefreshToken)
	if err != nil {
		log.Printf("Error revoking sessions: %v", err)
		return &authv1.RevokeAllSessionsResponse{
			Success: false,
			Message: "Failed to revoke sessions",
		}, nil
	}

	log.Printf("Revoked %d sessions for user: %s", revokedCount, claims.UserID)
	return &authv1.RevokeAllSessionsResponse{
		Success:      true,
		Message:      fmt.Sprintf("Successfully revoked %d other sessions", revokedCount),
		RevokedCount: revokedCount,
	}, nil
}

func (s *AuthServiceServerImpl) GetClientLoginActivity(ctx context.Context, req *authv1.GetClientLoginActivityRequest) (*authv1.GetClientLoginActivityResponse, error) {
	log.Printf("GetClientLoginActivity request received for client: %s", req.ClientId)

	if req.ClientId == "" {
		return &authv1.GetClientLoginActivityResponse{
			Success: false,
		}, nil
	}

	days := int(req.Days)
	if days <= 0 {
		days = 30
	}

	activity, err := s.repo.GetLoginActivityByClient(ctx, req.ClientId, days)
	if err != nil {
		log.Printf("Error fetching login activity: %v", err)
		return &authv1.GetClientLoginActivityResponse{
			Success: false,
		}, nil
	}

	dailyLogins := make([]*authv1.DailyLoginCount, len(activity))
	for i, a := range activity {
		dailyLogins[i] = &authv1.DailyLoginCount{
			Date:  a.Date,
			Count: a.Count,
		}
	}

	return &authv1.GetClientLoginActivityResponse{
		Success:     true,
		DailyLogins: dailyLogins,
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
