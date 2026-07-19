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

const refreshTokenLifetime = 7 * 24 * time.Hour // 7 days, sliding

type AuthServiceServerImpl struct {
	authv1.UnimplementedAuthServiceServer
	repo *repository.AuthRepository
}

func NewAuthServiceServer(db *gorm.DB) *AuthServiceServerImpl {
	return &AuthServiceServerImpl{
		repo: repository.NewAuthRepository(db),
	}
}

// authenticateClient verifies client_id + client_secret against the stored
// bcrypt hash. bcrypt comparison is constant-time. Every RPC (except
// HealthCheck and the ADMIN_SECRET-gated client management RPCs) must call
// this first. Suspended clients fail all RPC authentication.
func (s *AuthServiceServerImpl) authenticateClient(ctx context.Context, clientID, clientSecret string) (*models.Client, error) {
	return authenticateClientCreds(ctx, s.repo, clientID, clientSecret)
}

// authenticateClientCreds is the shared client-credential check, also used
// by PlatformService's machine RPC paths (Check/ListTuples with client
// credentials instead of a developer token).
func authenticateClientCreds(ctx context.Context, repo *repository.AuthRepository, clientID, clientSecret string) (*models.Client, error) {
	if clientID == "" || clientSecret == "" {
		return nil, fmt.Errorf("missing client credentials")
	}
	client, err := repo.GetClientByID(ctx, clientID)
	if err != nil {
		return nil, fmt.Errorf("client lookup failed: %w", err)
	}
	if !utils.CheckPasswordHash(clientSecret, client.ClientSecretHash) {
		return nil, fmt.Errorf("client secret mismatch")
	}
	if client.Suspended {
		return nil, fmt.Errorf("client is suspended")
	}
	return client, nil
}

// userScope resolves the identity scope a client's users live in:
// app-scoped clients own their user base; org-scoped clients share their
// org's user pool (SSO).
func userScope(client *models.Client) (scopeType, scopeID string) {
	if client.IdentityScope == models.ScopeOrg {
		return models.ScopeOrg, client.OrgID
	}
	return models.ScopeApp, client.ClientID
}

// authenticateUserToken validates an access token for a client and returns
// the claims plus the live session row. Fails if the session has been
// revoked (revocation-aware validation).
func (s *AuthServiceServerImpl) authenticateUserToken(ctx context.Context, accessToken, clientID string) (*utils.Claims, *models.Session, error) {
	claims, err := utils.ValidateJWTToken(accessToken)
	if err != nil {
		return nil, nil, fmt.Errorf("token validation failed: %w", err)
	}
	if claims.ClientID != clientID {
		return nil, nil, fmt.Errorf("token client mismatch")
	}
	session, err := s.repo.GetSessionByID(ctx, claims.SessionID)
	if err != nil {
		return nil, nil, fmt.Errorf("session lookup failed: %w", err)
	}
	if session.UserID != claims.Subject || session.ClientID != claims.ClientID {
		return nil, nil, fmt.Errorf("session does not match token claims")
	}
	return claims, session, nil
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
	log.Printf("RegisterUser request received for client: %s", req.ClientId)

	// Validation
	if err := s.validateUserRegistration(req); err != nil {
		return &authv1.RegisterUserResponse{
			Success: false,
			Message: err.Error(),
		}, nil
	}

	// Authenticate client
	client, err := s.authenticateClient(ctx, req.ClientId, req.ClientSecret)
	if err != nil {
		log.Printf("Client authentication failed for client %s: %v", req.ClientId, err)
		return &authv1.RegisterUserResponse{
			Success: false,
			Message: "Invalid client credentials",
		}, nil
	}

	// Check if email already exists within the client's identity scope
	scopeType, scopeID := userScope(client)
	emailExists, err := s.repo.IsEmailExists(ctx, req.Email, scopeType, scopeID)
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
		UserID:    userID,
		UserName:  req.Username,
		Email:     req.Email,
		Password:  hashedPassword,
		ScopeType: scopeType,
		ScopeID:   scopeID,
	}

	if err := s.repo.CreateUser(ctx, user); err != nil {
		log.Printf("Error creating user: %v", err)
		return &authv1.RegisterUserResponse{
			Success: false,
			Message: "Failed to create user",
		}, nil
	}

	log.Printf("User registered successfully: %s (client: %s)", userID, req.ClientId)
	return &authv1.RegisterUserResponse{
		Success: true,
		Message: "User registered successfully",
		UserId:  userID,
	}, nil
}

func (s *AuthServiceServerImpl) GetToken(ctx context.Context, req *authv1.GetTokenRequest) (*authv1.GetTokenResponse, error) {
	log.Printf("GetToken request received for client: %s", req.ClientId)

	// Validation
	if req.Email == "" || req.Password == "" || req.ClientId == "" || req.ClientSecret == "" {
		return &authv1.GetTokenResponse{
			Success: false,
			Message: "Email, password, client ID, and client secret are required",
		}, nil
	}

	// Authenticate client
	client, err := s.authenticateClient(ctx, req.ClientId, req.ClientSecret)
	if err != nil {
		log.Printf("Client authentication failed for client %s: %v", req.ClientId, err)
		return &authv1.GetTokenResponse{
			Success: false,
			Message: "Invalid client credentials",
		}, nil
	}

	return issueLoginTokens(ctx, s.repo, client, req.Email, req.Password, req.UserAgent)
}

// issueLoginTokens is the shared end-user login machinery used by GetToken
// and PlatformService.HostedLogin: resolves the user within the client's
// identity scope, verifies the password, creates a session (multi-session:
// one row per login/device), and returns the token pair + profile. The
// caller must already have established trust in the client (client secret
// for GetToken; the redirect_uri whitelist for HostedLogin).
func issueLoginTokens(ctx context.Context, repo *repository.AuthRepository, client *models.Client, email, password, userAgent string) (*authv1.GetTokenResponse, error) {
	// Resolve the user within the client's identity scope (app-scoped
	// clients have their own users; org-scoped clients share the org pool)
	scopeType, scopeID := userScope(client)
	user, err := repo.GetUserByEmail(ctx, email, scopeType, scopeID)
	if err != nil {
		log.Printf("Login failed for client %s: user lookup error", client.ClientID)
		return &authv1.GetTokenResponse{
			Success: false,
			Message: "Invalid credentials",
		}, nil
	}

	// Verify password
	if !utils.CheckPasswordHash(password, user.Password) {
		log.Printf("Login failed for user %s (client: %s)", user.UserID, client.ClientID)
		return &authv1.GetTokenResponse{
			Success: false,
			Message: "Invalid credentials",
		}, nil
	}

	// Create a new session (multi-session: one row per login/device)
	sessionID := utils.GenerateUUID()
	refreshToken, refreshHash, err := utils.GenerateRefreshToken(sessionID)
	if err != nil {
		log.Printf("Error generating refresh token: %v", err)
		return &authv1.GetTokenResponse{
			Success: false,
			Message: "Internal server error",
		}, nil
	}

	session := &models.Session{
		SessionID:        sessionID,
		UserID:           user.UserID,
		ClientID:         client.ClientID,
		RefreshTokenHash: refreshHash,
		UserAgent:        userAgent,
		ExpiresAt:        time.Now().Add(refreshTokenLifetime),
	}

	if err := repo.CreateSession(ctx, session); err != nil {
		log.Printf("Error creating session: %v", err)
		return &authv1.GetTokenResponse{
			Success: false,
			Message: "Internal server error",
		}, nil
	}

	// Generate JWT access token (sub, client_id, session_id)
	accessToken, expiresAt, err := utils.GenerateJWTToken(user.UserID, client.ClientID, sessionID)
	if err != nil {
		log.Printf("Error generating JWT token: %v", err)
		return &authv1.GetTokenResponse{
			Success: false,
			Message: "Internal server error",
		}, nil
	}

	userProfile := &authv1.UserProfile{
		UserId:    user.UserID,
		Username:  user.UserName,
		Email:     user.Email,
		ClientId:  client.ClientID,
		CreatedAt: timestamppb.New(user.CreatedAt),
	}

	log.Printf("User logged in successfully: %s (session: %s)", user.UserID, sessionID)
	return &authv1.GetTokenResponse{
		Success:      true,
		Message:      "Login successful",
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresAt:    timestamppb.New(expiresAt),
		User:         userProfile,
		SessionId:    sessionID,
	}, nil
}

func (s *AuthServiceServerImpl) ValidateToken(ctx context.Context, req *authv1.ValidateTokenRequest) (*authv1.ValidateTokenResponse, error) {
	log.Printf("ValidateToken request received for client: %s", req.ClientId)

	if req.AccessToken == "" {
		return &authv1.ValidateTokenResponse{
			Valid:   false,
			Message: "Access token is required",
		}, nil
	}

	// Authenticate client
	if _, err := s.authenticateClient(ctx, req.ClientId, req.ClientSecret); err != nil {
		log.Printf("Client authentication failed for client %s: %v", req.ClientId, err)
		return &authv1.ValidateTokenResponse{
			Valid:   false,
			Message: "Invalid client credentials",
		}, nil
	}

	// Validate token + revocation check (session row must exist)
	claims, session, err := s.authenticateUserToken(ctx, req.AccessToken, req.ClientId)
	if err != nil {
		log.Printf("Token validation failed for client %s: %v", req.ClientId, err)
		return &authv1.ValidateTokenResponse{
			Valid:   false,
			Message: "Invalid token",
		}, nil
	}

	// Check the user still exists
	user, err := s.repo.GetUserByID(ctx, claims.Subject)
	if err != nil {
		log.Printf("Error getting user by ID: %v", err)
		return &authv1.ValidateTokenResponse{
			Valid:   false,
			Message: "Invalid token",
		}, nil
	}

	userProfile := &authv1.UserProfile{
		UserId:    user.UserID,
		Username:  user.UserName,
		Email:     user.Email,
		ClientId:  claims.ClientID,
		CreatedAt: timestamppb.New(user.CreatedAt),
	}

	return &authv1.ValidateTokenResponse{
		Valid:     true,
		Message:   "Token is valid",
		UserId:    user.UserID,
		ExpiresAt: timestamppb.New(claims.ExpiresAt.Time),
		User:      userProfile,
		SessionId: session.SessionID,
	}, nil
}

func (s *AuthServiceServerImpl) RefreshToken(ctx context.Context, req *authv1.RefreshTokenRequest) (*authv1.RefreshTokenResponse, error) {
	log.Printf("RefreshToken request received for client: %s", req.ClientId)

	if req.RefreshToken == "" || req.ClientId == "" || req.ClientSecret == "" {
		return &authv1.RefreshTokenResponse{
			Success: false,
			Message: "Refresh token, client ID, and client secret are required",
		}, nil
	}

	// Authenticate client
	if _, err := s.authenticateClient(ctx, req.ClientId, req.ClientSecret); err != nil {
		log.Printf("Client authentication failed for client %s: %v", req.ClientId, err)
		return &authv1.RefreshTokenResponse{
			Success: false,
			Message: "Invalid client credentials",
		}, nil
	}

	// Parse opaque token: "<session_id>.<secret>"
	sessionID, secret, ok := utils.ParseRefreshToken(req.RefreshToken)
	if !ok {
		return &authv1.RefreshTokenResponse{
			Success: false,
			Message: "Invalid refresh token",
		}, nil
	}

	session, err := s.repo.GetSessionByID(ctx, sessionID)
	if err != nil {
		log.Printf("Refresh failed for client %s: session not found", req.ClientId)
		return &authv1.RefreshTokenResponse{
			Success: false,
			Message: "Invalid refresh token",
		}, nil
	}

	// Cross-tenant guard: session must belong to the calling client
	if session.ClientID != req.ClientId {
		log.Printf("Refresh failed: session client mismatch (client: %s)", req.ClientId)
		return &authv1.RefreshTokenResponse{
			Success: false,
			Message: "Invalid refresh token",
		}, nil
	}

	// Verify the secret against the stored bcrypt hash. A mismatch on an
	// existing session means a rotated (stale) token was replayed — treat
	// as theft and revoke the whole session.
	if !utils.VerifyRefreshSecret(secret, session.RefreshTokenHash) {
		log.Printf("Refresh token reuse detected; revoking session %s (user: %s, client: %s)", session.SessionID, session.UserID, session.ClientID)
		if err := s.repo.DeleteSessionByID(ctx, session.SessionID); err != nil {
			log.Printf("Error revoking session after reuse detection: %v", err)
		}
		return &authv1.RefreshTokenResponse{
			Success: false,
			Message: "Invalid refresh token",
		}, nil
	}

	// Rotate: new secret, new hash, sliding expiry
	newRefreshToken, newHash, err := utils.GenerateRefreshToken(session.SessionID)
	if err != nil {
		log.Printf("Error generating refresh token: %v", err)
		return &authv1.RefreshTokenResponse{
			Success: false,
			Message: "Internal server error",
		}, nil
	}

	session.RefreshTokenHash = newHash
	session.ExpiresAt = time.Now().Add(refreshTokenLifetime)
	if err := s.repo.UpdateSession(ctx, session); err != nil {
		log.Printf("Error updating session: %v", err)
		return &authv1.RefreshTokenResponse{
			Success: false,
			Message: "Internal server error",
		}, nil
	}

	accessToken, expiresAt, err := utils.GenerateJWTToken(session.UserID, session.ClientID, session.SessionID)
	if err != nil {
		log.Printf("Error generating JWT token: %v", err)
		return &authv1.RefreshTokenResponse{
			Success: false,
			Message: "Internal server error",
		}, nil
	}

	log.Printf("Token refreshed successfully for user: %s (session: %s)", session.UserID, session.SessionID)
	return &authv1.RefreshTokenResponse{
		Success:      true,
		Message:      "Token refreshed successfully",
		AccessToken:  accessToken,
		RefreshToken: newRefreshToken,
		ExpiresAt:    timestamppb.New(expiresAt),
	}, nil
}

func (s *AuthServiceServerImpl) RevokeToken(ctx context.Context, req *authv1.RevokeTokenRequest) (*authv1.RevokeTokenResponse, error) {
	log.Printf("RevokeToken request received for client: %s", req.ClientId)

	if req.RefreshToken == "" {
		return &authv1.RevokeTokenResponse{
			Success: false,
			Message: "Refresh token is required",
		}, nil
	}

	// Authenticate client
	if _, err := s.authenticateClient(ctx, req.ClientId, req.ClientSecret); err != nil {
		log.Printf("Client authentication failed for client %s: %v", req.ClientId, err)
		return &authv1.RevokeTokenResponse{
			Success: false,
			Message: "Invalid client credentials",
		}, nil
	}

	sessionID, secret, ok := utils.ParseRefreshToken(req.RefreshToken)
	if !ok {
		return &authv1.RevokeTokenResponse{
			Success: false,
			Message: "Invalid refresh token",
		}, nil
	}

	session, err := s.repo.GetSessionByID(ctx, sessionID)
	if err != nil {
		return &authv1.RevokeTokenResponse{
			Success: false,
			Message: "Invalid refresh token",
		}, nil
	}

	if session.ClientID != req.ClientId || !utils.VerifyRefreshSecret(secret, session.RefreshTokenHash) {
		return &authv1.RevokeTokenResponse{
			Success: false,
			Message: "Invalid refresh token",
		}, nil
	}

	if err := s.repo.DeleteSessionByID(ctx, session.SessionID); err != nil {
		log.Printf("Error deleting session: %v", err)
		return &authv1.RevokeTokenResponse{
			Success: false,
			Message: "Internal server error",
		}, nil
	}

	log.Printf("Session revoked successfully: %s (user: %s)", session.SessionID, session.UserID)
	return &authv1.RevokeTokenResponse{
		Success: true,
		Message: "Token revoked successfully",
	}, nil
}

func (s *AuthServiceServerImpl) LogoutAllSessions(ctx context.Context, req *authv1.LogoutAllSessionsRequest) (*authv1.LogoutAllSessionsResponse, error) {
	log.Printf("LogoutAllSessions request received for client: %s", req.ClientId)

	// Authenticate client
	if _, err := s.authenticateClient(ctx, req.ClientId, req.ClientSecret); err != nil {
		log.Printf("Client authentication failed for client %s: %v", req.ClientId, err)
		return &authv1.LogoutAllSessionsResponse{
			Success: false,
			Message: "Invalid client credentials",
		}, nil
	}

	claims, _, err := s.authenticateUserToken(ctx, req.AccessToken, req.ClientId)
	if err != nil {
		log.Printf("Token validation failed for client %s: %v", req.ClientId, err)
		return &authv1.LogoutAllSessionsResponse{
			Success: false,
			Message: "Invalid token",
		}, nil
	}

	revoked, err := s.repo.DeleteUserClientSessions(ctx, claims.Subject, req.ClientId)
	if err != nil {
		log.Printf("Error deleting sessions for user %s: %v", claims.Subject, err)
		return &authv1.LogoutAllSessionsResponse{
			Success: false,
			Message: "Internal server error",
		}, nil
	}

	log.Printf("All sessions revoked for user: %s (client: %s, count: %d)", claims.Subject, req.ClientId, revoked)
	return &authv1.LogoutAllSessionsResponse{
		Success:      true,
		Message:      "All sessions logged out",
		RevokedCount: int32(revoked),
	}, nil
}

func (s *AuthServiceServerImpl) GetUserSessions(ctx context.Context, req *authv1.GetUserSessionsRequest) (*authv1.GetUserSessionsResponse, error) {
	log.Printf("GetUserSessions request received for client: %s", req.ClientId)

	// Authenticate client
	if _, err := s.authenticateClient(ctx, req.ClientId, req.ClientSecret); err != nil {
		log.Printf("Client authentication failed for client %s: %v", req.ClientId, err)
		return &authv1.GetUserSessionsResponse{
			Success: false,
			Message: "Invalid client credentials",
		}, nil
	}

	claims, current, err := s.authenticateUserToken(ctx, req.AccessToken, req.ClientId)
	if err != nil {
		log.Printf("Token validation failed for client %s: %v", req.ClientId, err)
		return &authv1.GetUserSessionsResponse{
			Success: false,
			Message: "Invalid token",
		}, nil
	}

	sessions, err := s.repo.GetSessionsByUserAndClient(ctx, claims.Subject, req.ClientId)
	if err != nil {
		log.Printf("Error listing sessions for user %s: %v", claims.Subject, err)
		return &authv1.GetUserSessionsResponse{
			Success: false,
			Message: "Internal server error",
		}, nil
	}

	infos := make([]*authv1.SessionInfo, 0, len(sessions))
	for _, sess := range sessions {
		infos = append(infos, &authv1.SessionInfo{
			SessionId: sess.SessionID,
			UserAgent: sess.UserAgent,
			CreatedAt: timestamppb.New(sess.CreatedAt),
			ExpiresAt: timestamppb.New(sess.ExpiresAt),
			Current:   sess.SessionID == current.SessionID,
		})
	}

	return &authv1.GetUserSessionsResponse{
		Success:  true,
		Message:  "Sessions retrieved successfully",
		Sessions: infos,
	}, nil
}

func (s *AuthServiceServerImpl) RevokeSession(ctx context.Context, req *authv1.RevokeSessionRequest) (*authv1.RevokeSessionResponse, error) {
	log.Printf("RevokeSession request received for client: %s", req.ClientId)

	if req.SessionId == "" {
		return &authv1.RevokeSessionResponse{
			Success: false,
			Message: "Session ID is required",
		}, nil
	}

	// Authenticate client
	if _, err := s.authenticateClient(ctx, req.ClientId, req.ClientSecret); err != nil {
		log.Printf("Client authentication failed for client %s: %v", req.ClientId, err)
		return &authv1.RevokeSessionResponse{
			Success: false,
			Message: "Invalid client credentials",
		}, nil
	}

	claims, _, err := s.authenticateUserToken(ctx, req.AccessToken, req.ClientId)
	if err != nil {
		log.Printf("Token validation failed for client %s: %v", req.ClientId, err)
		return &authv1.RevokeSessionResponse{
			Success: false,
			Message: "Invalid token",
		}, nil
	}

	// The target session must belong to the token's user and client
	target, err := s.repo.GetSessionByID(ctx, req.SessionId)
	if err != nil || target.UserID != claims.Subject || target.ClientID != req.ClientId {
		return &authv1.RevokeSessionResponse{
			Success: false,
			Message: "Session not found",
		}, nil
	}

	if err := s.repo.DeleteSessionByID(ctx, target.SessionID); err != nil {
		log.Printf("Error deleting session %s: %v", target.SessionID, err)
		return &authv1.RevokeSessionResponse{
			Success: false,
			Message: "Internal server error",
		}, nil
	}

	log.Printf("Session revoked: %s (user: %s, client: %s)", target.SessionID, claims.Subject, req.ClientId)
	return &authv1.RevokeSessionResponse{
		Success: true,
		Message: "Session revoked successfully",
	}, nil
}

func (s *AuthServiceServerImpl) RegisterClient(ctx context.Context, req *authv1.RegisterClientRequest) (*authv1.RegisterClientResponse, error) {
	log.Printf("RegisterClient request received")

	if !utils.CheckAdminSecret(req.AdminSecret) {
		log.Printf("RegisterClient rejected: admin secret mismatch")
		return &authv1.RegisterClientResponse{
			Success: false,
			Message: "Invalid admin credentials",
		}, nil
	}

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

	secretHash, err := utils.HashPassword(clientSecret)
	if err != nil {
		log.Printf("Error hashing client secret: %v", err)
		return &authv1.RegisterClientResponse{
			Success: false,
			Message: "Internal server error",
		}, nil
	}

	// Create client (only the bcrypt hash is stored)
	client := &models.Client{
		ClientID:         clientID,
		ClientName:       req.ClientName,
		ClientSecretHash: secretHash,
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

func (s *AuthServiceServerImpl) ChangeUserPassword(ctx context.Context, req *authv1.ChangeUserPasswordRequest) (*authv1.ChangeUserPasswordResponse, error) {
	log.Printf("ChangePassword request received for client: %s", req.ClientId)

	if req.AccessToken == "" || req.CurrentPassword == "" || req.NewPassword == "" {
		return &authv1.ChangeUserPasswordResponse{
			Success: false,
			Message: "Access token, current password, and new password are required",
		}, nil
	}

	// Same policy as registration
	if len(req.NewPassword) < 8 {
		return &authv1.ChangeUserPasswordResponse{
			Success: false,
			Message: "Password must be at least 8 characters long",
		}, nil
	}

	// Authenticate client
	if _, err := s.authenticateClient(ctx, req.ClientId, req.ClientSecret); err != nil {
		log.Printf("Client authentication failed for client %s: %v", req.ClientId, err)
		return &authv1.ChangeUserPasswordResponse{
			Success: false,
			Message: "Invalid client credentials",
		}, nil
	}

	// Validate access token (revocation-aware)
	claims, currentSession, err := s.authenticateUserToken(ctx, req.AccessToken, req.ClientId)
	if err != nil {
		log.Printf("Token validation failed for client %s: %v", req.ClientId, err)
		return &authv1.ChangeUserPasswordResponse{
			Success: false,
			Message: "Invalid access token",
		}, nil
	}

	// Get user
	user, err := s.repo.GetUserByID(ctx, claims.Subject)
	if err != nil {
		log.Printf("Error getting user by ID: %v", err)
		return &authv1.ChangeUserPasswordResponse{
			Success: false,
			Message: "Invalid access token",
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

	// Invalidate all OTHER sessions; the current session stays alive
	if err := s.repo.DeleteOtherUserSessions(ctx, user.UserID, currentSession.SessionID); err != nil {
		log.Printf("Error invalidating other user sessions: %v", err)
		return &authv1.ChangeUserPasswordResponse{
			Success: false,
			Message: "Password changed but failed to invalidate other sessions",
		}, nil
	}

	log.Printf("Password changed successfully for user: %s", user.UserID)
	return &authv1.ChangeUserPasswordResponse{
		Success: true,
		Message: "Password changed successfully. Other sessions have been logged out.",
	}, nil
}

func (s *AuthServiceServerImpl) ChangeClientSecret(ctx context.Context, req *authv1.ChangeClientSecretRequest) (*authv1.ChangeClientSecretResponse, error) {
	log.Printf("ChangeClientSecret request received for client: %s", req.ClientId)

	if !utils.CheckAdminSecret(req.AdminSecret) {
		log.Printf("ChangeClientSecret rejected: admin secret mismatch")
		return &authv1.ChangeClientSecretResponse{Success: false, Message: "Invalid admin credentials"}, nil
	}

	if req.ClientId == "" || req.CurrentSecret == "" {
		return &authv1.ChangeClientSecretResponse{Success: false, Message: "client_id and current_secret are required"}, nil
	}

	// Validate current secret
	if _, err := s.authenticateClient(ctx, req.ClientId, req.CurrentSecret); err != nil {
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

	newSecretHash, err := utils.HashPassword(newSecret)
	if err != nil {
		log.Printf("Error hashing client secret: %v", err)
		return &authv1.ChangeClientSecretResponse{Success: false, Message: "Internal server error"}, nil
	}

	if err := s.repo.UpdateClientSecretHash(ctx, req.ClientId, newSecretHash); err != nil {
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

// Helper functions
func (s *AuthServiceServerImpl) validateUserRegistration(req *authv1.RegisterUserRequest) error {
	if req.Username == "" {
		return fmt.Errorf("username is required")
	}

	if req.Email == "" {
		return fmt.Errorf("email is required")
	}

	if !isValidEmail(req.Email) {
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

func isValidEmail(email string) bool {
	email = strings.TrimSpace(email)
	emailRegex := regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)
	return emailRegex.MatchString(email)
}
