package service

import (
	"context"
	"os"
	"testing"
	"time"

	"authservice/pkg/models"
	"authservice/pkg/repository"
	authv1 "authservice/proto/auth/v1"

	"authservice/pkg/utils"

	sqlite "github.com/glebarez/sqlite"
	"google.golang.org/protobuf/types/known/emptypb"
	"gorm.io/gorm"
)

func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open in-memory sqlite: %v", err)
	}
	if err := db.AutoMigrate(models.GetAllModels()...); err != nil {
		t.Fatalf("failed to automigrate: %v", err)
	}
	return db
}

func seedClient(t *testing.T, db *gorm.DB, clientID string) {
	t.Helper()
	repo := repository.NewAuthRepository(db)
	if err := repo.CreateClient(context.Background(), &models.Client{
		ClientID:     clientID,
		ClientName:   "test-client",
		ClientSecret: "secret",
	}); err != nil {
		t.Fatalf("failed to seed client: %v", err)
	}
}

func seedUser(t *testing.T, db *gorm.DB, userID, clientID, email, username, rawPassword string) *models.User {
	t.Helper()
	repo := repository.NewAuthRepository(db)
	hashed, err := utils.HashPassword(rawPassword)
	if err != nil {
		t.Fatalf("failed to hash password: %v", err)
	}
	user := &models.User{
		UserID:   userID,
		UserName: username,
		Email:    email,
		Password: hashed,
		ClientID: clientID,
	}
	if err := repo.CreateUser(context.Background(), user); err != nil {
		t.Fatalf("failed to seed user: %v", err)
	}
	return user
}

func seedSession(t *testing.T, db *gorm.DB, userID, clientID, refreshToken string, expiresAt time.Time) {
	t.Helper()
	repo := repository.NewAuthRepository(db)
	if err := repo.CreateOrUpdateSession(context.Background(), &models.Session{
		UserID:       userID,
		ClientID:     clientID,
		RefreshToken: refreshToken,
		UserAgent:    "test-agent",
		ExpiresAt:    expiresAt,
	}); err != nil {
		t.Fatalf("failed to seed session: %v", err)
	}
}

func withJWTSecret(t *testing.T) func() {
	t.Helper()
	prev := os.Getenv("JWT_SECRET")
	if err := os.Setenv("JWT_SECRET", "test-secret"); err != nil {
		t.Fatalf("failed to set JWT_SECRET: %v", err)
	}
	return func() {
		_ = os.Setenv("JWT_SECRET", prev)
	}
}

func TestHealthCheck(t *testing.T) {
	db := newTestDB(t)
	svc := NewAuthServiceServer(db)

	resp, err := svc.HealthCheck(context.Background(), &emptypb.Empty{})
	if err != nil {
		t.Fatalf("HealthCheck returned error: %v", err)
	}
	if resp.Status != authv1.HealthCheckResponse_SERVING {
		t.Fatalf("unexpected status: %v", resp.Status)
	}
}

func TestRegisterUser_Success(t *testing.T) {
	db := newTestDB(t)
	svc := NewAuthServiceServer(db)
	seedClient(t, db, "client-1")

	resp, err := svc.RegisterUser(context.Background(), &authv1.RegisterUserRequest{
		Username: "alice",
		Email:    "alice@example.com",
		Password: "password123",
		ClientId: "client-1",
	})
	if err != nil {
		t.Fatalf("RegisterUser returned error: %v", err)
	}
	if !resp.Success {
		t.Fatalf("expected success, got: %v (%s)", resp.Success, resp.Message)
	}
	if resp.UserId == "" {
		t.Fatalf("expected user_id to be set")
	}
}

func TestRegisterUser_InvalidClient(t *testing.T) {
	db := newTestDB(t)
	svc := NewAuthServiceServer(db)

	resp, _ := svc.RegisterUser(context.Background(), &authv1.RegisterUserRequest{
		Username: "bob",
		Email:    "bob@example.com",
		Password: "password123",
		ClientId: "non-existent",
	})
	if resp.Success {
		t.Fatalf("expected failure for invalid client")
	}
}

func TestLoginUser_Success(t *testing.T) {
	cleanup := withJWTSecret(t)
	defer cleanup()

	db := newTestDB(t)
	svc := NewAuthServiceServer(db)
	seedClient(t, db, "client-1")
	seedUser(t, db, "user-1", "client-1", "alice@example.com", "alice", "password123")

	resp, err := svc.GetToken(context.Background(), &authv1.GetTokenRequest{
		Email:     "alice@example.com",
		Password:  "password123",
		ClientId:  "client-1",
		UserAgent: "unit-test",
	})
	if err != nil {
		t.Fatalf("LoginUser returned error: %v", err)
	}
	if !resp.Success || resp.AccessToken == "" || resp.RefreshToken == "" {
		t.Fatalf("expected successful login with tokens, got success=%v msg=%s", resp.Success, resp.Message)
	}

	// ensure session persisted
	repo := repository.NewAuthRepository(db)
	if _, err := repo.GetSessionByUserAndClient(context.Background(), "user-1", "client-1"); err != nil {
		t.Fatalf("expected session to be created: %v", err)
	}
}

func TestValidateToken_Success(t *testing.T) {
	cleanup := withJWTSecret(t)
	defer cleanup()

	db := newTestDB(t)
	svc := NewAuthServiceServer(db)
	seedClient(t, db, "client-1")
	user := seedUser(t, db, "user-1", "client-1", "alice@example.com", "alice", "password123")

	refresh := "refresh-abc"
	seedSession(t, db, user.UserID, user.ClientID, refresh, time.Now().Add(24*time.Hour))

	token, _, err := utils.GenerateJWTToken(user.UserID, user.UserName, user.ClientID, refresh)
	if err != nil {
		t.Fatalf("failed to generate jwt: %v", err)
	}

	resp, err := svc.ValidateToken(context.Background(), &authv1.ValidateTokenRequest{AccessToken: token})
	if err != nil {
		t.Fatalf("ValidateToken returned error: %v", err)
	}
	if !resp.Valid || resp.UserId != user.UserID {
		t.Fatalf("expected valid token for user, got valid=%v user_id=%s msg=%s", resp.Valid, resp.UserId, resp.Message)
	}
}

func TestRefreshToken_Success(t *testing.T) {
	cleanup := withJWTSecret(t)
	defer cleanup()

	db := newTestDB(t)
	svc := NewAuthServiceServer(db)
	seedClient(t, db, "client-1")
	user := seedUser(t, db, "user-1", "client-1", "alice@example.com", "alice", "password123")

	oldRefresh := "refresh-old"
	seedSession(t, db, user.UserID, user.ClientID, oldRefresh, time.Now().Add(24*time.Hour))

	resp, err := svc.RefreshToken(context.Background(), &authv1.RefreshTokenRequest{
		RefreshToken: oldRefresh,
		ClientId:     user.ClientID,
	})
	if err != nil {
		t.Fatalf("RefreshToken returned error: %v", err)
	}
	if !resp.Success || resp.RefreshToken == "" || resp.RefreshToken == oldRefresh {
		t.Fatalf("expected success with a new refresh token, got success=%v msg=%s", resp.Success, resp.Message)
	}
}

func TestLogoutUser_Success(t *testing.T) {
	db := newTestDB(t)
	svc := NewAuthServiceServer(db)
	seedClient(t, db, "client-1")
	user := seedUser(t, db, "user-1", "client-1", "alice@example.com", "alice", "password123")

	refresh := "refresh-to-delete"
	seedSession(t, db, user.UserID, user.ClientID, refresh, time.Now().Add(24*time.Hour))

	resp, err := svc.RevokeToken(context.Background(), &authv1.RevokeTokenRequest{RefreshToken: refresh})
	if err != nil {
		t.Fatalf("LogoutUser returned error: %v", err)
	}
	if !resp.Success {
		t.Fatalf("expected success, got msg=%s", resp.Message)
	}

	// assert session removed
	repo := repository.NewAuthRepository(db)
	if _, err := repo.GetSessionByRefreshToken(context.Background(), refresh); err == nil {
		t.Fatalf("expected session to be deleted")
	}
}

func TestGetUserProfile_Success(t *testing.T) {
	cleanup := withJWTSecret(t)
	defer cleanup()

	db := newTestDB(t)
	svc := NewAuthServiceServer(db)
	seedClient(t, db, "client-1")
	user := seedUser(t, db, "user-1", "client-1", "alice@example.com", "alice", "password123")

	// Seed a matching session to satisfy ValidateToken's session check
	refresh := "any-refresh"
	seedSession(t, db, user.UserID, user.ClientID, refresh, time.Now().Add(24*time.Hour))

	token, _, err := utils.GenerateJWTToken(user.UserID, user.UserName, user.ClientID, refresh)
	if err != nil {
		t.Fatalf("failed to generate jwt: %v", err)
	}

	resp, err := svc.ValidateToken(context.Background(), &authv1.ValidateTokenRequest{AccessToken: token})
	if err != nil {
		t.Fatalf("ValidateToken returned error: %v", err)
	}
	if !resp.Valid || resp.User == nil || resp.User.UserId != user.UserID {
		t.Fatalf("expected valid token with user, got valid=%v", resp.Valid)
	}
}

func TestChangePassword_Success(t *testing.T) {
	cleanup := withJWTSecret(t)
	defer cleanup()

	db := newTestDB(t)
	svc := NewAuthServiceServer(db)
	seedClient(t, db, "client-1")
	user := seedUser(t, db, "user-1", "client-1", "alice@example.com", "alice", "old-password")

	// seed a session that should be invalidated
	seedSession(t, db, user.UserID, user.ClientID, "refresh-to-be-removed", time.Now().Add(24*time.Hour))

	token, _, err := utils.GenerateJWTToken(user.UserID, user.UserName, user.ClientID, "refresh-to-be-removed")
	if err != nil {
		t.Fatalf("failed to generate jwt: %v", err)
	}

	resp, err := svc.ChangeUserPassword(context.Background(), &authv1.ChangeUserPasswordRequest{
		AccessToken:     token,
		CurrentPassword: "old-password",
		NewPassword:     "new-password-123",
	})
	if err != nil {
		t.Fatalf("ChangePassword returned error: %v", err)
	}
	if !resp.Success {
		t.Fatalf("expected success changing password, got msg=%s", resp.Message)
	}

	// ensure sessions invalidated
	repo := repository.NewAuthRepository(db)
	if _, err := repo.GetSessionByUserAndClient(context.Background(), user.UserID, user.ClientID); err == nil {
		t.Fatalf("expected user sessions to be deleted")
	}
}
