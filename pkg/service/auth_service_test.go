package service

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"authservice/pkg/models"
	"authservice/pkg/repository"
	authv1 "authservice/proto/auth/v1"

	"authservice/pkg/utils"

	sqlite "github.com/glebarez/sqlite"
	"github.com/golang-jwt/jwt/v5"
	"google.golang.org/protobuf/types/known/emptypb"
	"gorm.io/gorm"
)

const testClientSecret = "test-client-secret"

// TestMain pins JWT_PRIVATE_KEY_FILE to a temp path so utils generates a
// dev key there; tests read the same PEM back to craft expired/tampered
// tokens against the same keypair.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "authsvc-test")
	if err != nil {
		panic(err)
	}
	os.Setenv("JWT_PRIVATE_KEY_FILE", filepath.Join(dir, "jwt_test_key.pem"))
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

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

// seedClient stores the bcrypt hash of testClientSecret for the client.
func seedClient(t *testing.T, db *gorm.DB, clientID string) {
	t.Helper()
	repo := repository.NewAuthRepository(db)
	hash, err := utils.HashPassword(testClientSecret)
	if err != nil {
		t.Fatalf("failed to hash client secret: %v", err)
	}
	if err := repo.CreateClient(context.Background(), &models.Client{
		ClientID:         clientID,
		ClientName:       "test-client",
		ClientSecretHash: hash,
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

// login performs a full GetToken flow and asserts success.
func login(t *testing.T, svc *AuthServiceServerImpl, email, password, clientID string) *authv1.GetTokenResponse {
	t.Helper()
	resp, err := svc.GetToken(context.Background(), &authv1.GetTokenRequest{
		Email:        email,
		Password:     password,
		ClientId:     clientID,
		ClientSecret: testClientSecret,
		UserAgent:    "unit-test",
	})
	if err != nil {
		t.Fatalf("GetToken returned error: %v", err)
	}
	if !resp.Success {
		t.Fatalf("expected successful login, got msg=%s", resp.Message)
	}
	return resp
}

// loadTestSigningKey reads the dev key utils generated at
// JWT_PRIVATE_KEY_FILE, so tests can mint custom (e.g. expired) tokens
// signed by the real key.
func loadTestSigningKey(t *testing.T) interface{} {
	t.Helper()
	// Force key creation first
	if _, _, err := utils.GenerateJWTToken("boot", "boot", "boot"); err != nil {
		t.Fatalf("failed to bootstrap signing key: %v", err)
	}
	data, err := os.ReadFile(os.Getenv("JWT_PRIVATE_KEY_FILE"))
	if err != nil {
		t.Fatalf("failed to read test signing key: %v", err)
	}
	block, _ := pem.Decode(data)
	key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		t.Fatalf("failed to parse test signing key: %v", err)
	}
	return key
}

func mintToken(t *testing.T, key interface{}, userID, clientID, sessionID string, expiresAt time.Time) string {
	t.Helper()
	claims := jwt.MapClaims{
		"sub":        userID,
		"client_id":  clientID,
		"session_id": sessionID,
		"iat":        time.Now().Add(-time.Hour).Unix(),
		"exp":        expiresAt.Unix(),
		"iss":        "auth-service",
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodRS256, claims).SignedString(key)
	if err != nil {
		t.Fatalf("failed to mint token: %v", err)
	}
	return token
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
		Username:     "alice",
		Email:        "alice@example.com",
		Password:     "password123",
		ClientId:     "client-1",
		ClientSecret: testClientSecret,
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

func TestRegisterUser_InvalidClientSecret(t *testing.T) {
	db := newTestDB(t)
	svc := NewAuthServiceServer(db)
	seedClient(t, db, "client-1")

	resp, _ := svc.RegisterUser(context.Background(), &authv1.RegisterUserRequest{
		Username:     "bob",
		Email:        "bob@example.com",
		Password:     "password123",
		ClientId:     "client-1",
		ClientSecret: "wrong-secret",
	})
	if resp.Success {
		t.Fatalf("expected failure for invalid client secret")
	}
}

func TestRegisterUser_InvalidClient(t *testing.T) {
	db := newTestDB(t)
	svc := NewAuthServiceServer(db)

	resp, _ := svc.RegisterUser(context.Background(), &authv1.RegisterUserRequest{
		Username:     "bob",
		Email:        "bob@example.com",
		Password:     "password123",
		ClientId:     "non-existent",
		ClientSecret: testClientSecret,
	})
	if resp.Success {
		t.Fatalf("expected failure for invalid client")
	}
}

func TestRegisterUser_SameEmailAcrossClients(t *testing.T) {
	db := newTestDB(t)
	svc := NewAuthServiceServer(db)
	seedClient(t, db, "client-a")
	seedClient(t, db, "client-b")

	for _, clientID := range []string{"client-a", "client-b"} {
		resp, err := svc.RegisterUser(context.Background(), &authv1.RegisterUserRequest{
			Username:     "alice",
			Email:        "alice@example.com",
			Password:     "password123",
			ClientId:     clientID,
			ClientSecret: testClientSecret,
		})
		if err != nil || !resp.Success {
			t.Fatalf("expected same email to register under %s, got msg=%s", clientID, resp.Message)
		}
	}

	// Duplicate within the same client must still fail
	resp, _ := svc.RegisterUser(context.Background(), &authv1.RegisterUserRequest{
		Username:     "alice2",
		Email:        "alice@example.com",
		Password:     "password123",
		ClientId:     "client-a",
		ClientSecret: testClientSecret,
	})
	if resp.Success {
		t.Fatalf("expected duplicate email within the same client to fail")
	}
}

func TestLoginUser_Success(t *testing.T) {
	db := newTestDB(t)
	svc := NewAuthServiceServer(db)
	seedClient(t, db, "client-1")
	seedUser(t, db, "user-1", "client-1", "alice@example.com", "alice", "password123")

	resp := login(t, svc, "alice@example.com", "password123", "client-1")
	if resp.AccessToken == "" || resp.RefreshToken == "" || resp.SessionId == "" {
		t.Fatalf("expected tokens and session_id in login response")
	}
	if !strings.HasPrefix(resp.RefreshToken, resp.SessionId+".") {
		t.Fatalf("expected refresh token to be prefixed with session_id lookup key")
	}

	// ensure session persisted
	repo := repository.NewAuthRepository(db)
	if _, err := repo.GetSessionByID(context.Background(), resp.SessionId); err != nil {
		t.Fatalf("expected session to be created: %v", err)
	}
}

func TestLogin_MultiSession(t *testing.T) {
	db := newTestDB(t)
	svc := NewAuthServiceServer(db)
	seedClient(t, db, "client-1")
	seedUser(t, db, "user-1", "client-1", "alice@example.com", "alice", "password123")

	first := login(t, svc, "alice@example.com", "password123", "client-1")
	second := login(t, svc, "alice@example.com", "password123", "client-1")
	if first.SessionId == second.SessionId {
		t.Fatalf("expected two logins to create distinct sessions")
	}

	// Both sessions' tokens must validate
	for _, tok := range []string{first.AccessToken, second.AccessToken} {
		resp, _ := svc.ValidateToken(context.Background(), &authv1.ValidateTokenRequest{
			AccessToken:  tok,
			ClientId:     "client-1",
			ClientSecret: testClientSecret,
		})
		if !resp.Valid {
			t.Fatalf("expected both concurrent sessions to be valid: %s", resp.Message)
		}
	}
}

func TestValidateToken_Success(t *testing.T) {
	db := newTestDB(t)
	svc := NewAuthServiceServer(db)
	seedClient(t, db, "client-1")
	seedUser(t, db, "user-1", "client-1", "alice@example.com", "alice", "password123")

	loginResp := login(t, svc, "alice@example.com", "password123", "client-1")

	resp, err := svc.ValidateToken(context.Background(), &authv1.ValidateTokenRequest{
		AccessToken:  loginResp.AccessToken,
		ClientId:     "client-1",
		ClientSecret: testClientSecret,
	})
	if err != nil {
		t.Fatalf("ValidateToken returned error: %v", err)
	}
	if !resp.Valid || resp.UserId != "user-1" || resp.SessionId != loginResp.SessionId {
		t.Fatalf("expected valid token, got valid=%v user_id=%s msg=%s", resp.Valid, resp.UserId, resp.Message)
	}
	if resp.User == nil || resp.User.UserId != "user-1" {
		t.Fatalf("expected user profile in response")
	}
}

// Token misuse: expired, tampered, cross-client, revoked.
func TestValidateToken_Misuse(t *testing.T) {
	db := newTestDB(t)
	svc := NewAuthServiceServer(db)
	seedClient(t, db, "client-a")
	seedClient(t, db, "client-b")
	seedUser(t, db, "user-1", "client-a", "alice@example.com", "alice", "password123")

	loginResp := login(t, svc, "alice@example.com", "password123", "client-a")
	key := loadTestSigningKey(t)

	validate := func(token, clientID string) *authv1.ValidateTokenResponse {
		resp, err := svc.ValidateToken(context.Background(), &authv1.ValidateTokenRequest{
			AccessToken:  token,
			ClientId:     clientID,
			ClientSecret: testClientSecret,
		})
		if err != nil {
			t.Fatalf("ValidateToken returned error: %v", err)
		}
		return resp
	}

	t.Run("expired", func(t *testing.T) {
		expired := mintToken(t, key, "user-1", "client-a", loginResp.SessionId, time.Now().Add(-time.Minute))
		if resp := validate(expired, "client-a"); resp.Valid {
			t.Fatalf("expected expired token to be rejected")
		}
	})

	t.Run("tampered", func(t *testing.T) {
		parts := strings.Split(loginResp.AccessToken, ".")
		tampered := parts[0] + "." + parts[1] + "x." + parts[2]
		if resp := validate(tampered, "client-a"); resp.Valid {
			t.Fatalf("expected tampered token to be rejected")
		}
	})

	t.Run("cross-client", func(t *testing.T) {
		// client-a's token presented with client-b's credentials
		if resp := validate(loginResp.AccessToken, "client-b"); resp.Valid {
			t.Fatalf("expected token issued for another client to be rejected")
		}
	})

	t.Run("revoked", func(t *testing.T) {
		repo := repository.NewAuthRepository(db)
		if err := repo.DeleteSessionByID(context.Background(), loginResp.SessionId); err != nil {
			t.Fatalf("failed to revoke session: %v", err)
		}
		if resp := validate(loginResp.AccessToken, "client-a"); resp.Valid {
			t.Fatalf("expected token of revoked session to be rejected")
		}
	})
}

// Cross-tenant isolation across the main RPCs.
func TestCrossTenantIsolation(t *testing.T) {
	db := newTestDB(t)
	svc := NewAuthServiceServer(db)
	seedClient(t, db, "client-a")
	seedClient(t, db, "client-b")
	seedUser(t, db, "user-a", "client-a", "alice@example.com", "alice", "password123")

	t.Run("login against wrong client", func(t *testing.T) {
		resp, _ := svc.GetToken(context.Background(), &authv1.GetTokenRequest{
			Email:        "alice@example.com",
			Password:     "password123",
			ClientId:     "client-b",
			ClientSecret: testClientSecret,
		})
		if resp.Success {
			t.Fatalf("expected client-a user login via client-b to fail")
		}
	})

	loginResp := login(t, svc, "alice@example.com", "password123", "client-a")

	t.Run("refresh with wrong client", func(t *testing.T) {
		resp, _ := svc.RefreshToken(context.Background(), &authv1.RefreshTokenRequest{
			RefreshToken: loginResp.RefreshToken,
			ClientId:     "client-b",
			ClientSecret: testClientSecret,
		})
		if resp.Success {
			t.Fatalf("expected refresh of client-a session via client-b to fail")
		}
	})

	t.Run("revoke with wrong client", func(t *testing.T) {
		resp, _ := svc.RevokeToken(context.Background(), &authv1.RevokeTokenRequest{
			RefreshToken: loginResp.RefreshToken,
			ClientId:     "client-b",
			ClientSecret: testClientSecret,
		})
		if resp.Success {
			t.Fatalf("expected revoke of client-a session via client-b to fail")
		}
		// session must still exist
		repo := repository.NewAuthRepository(db)
		if _, err := repo.GetSessionByID(context.Background(), loginResp.SessionId); err != nil {
			t.Fatalf("expected client-a session to survive client-b revoke attempt")
		}
	})

	t.Run("sessions RPCs with wrong client", func(t *testing.T) {
		sessResp, _ := svc.GetUserSessions(context.Background(), &authv1.GetUserSessionsRequest{
			AccessToken:  loginResp.AccessToken,
			ClientId:     "client-b",
			ClientSecret: testClientSecret,
		})
		if sessResp.Success {
			t.Fatalf("expected GetUserSessions via client-b to fail")
		}
		revokeResp, _ := svc.RevokeSession(context.Background(), &authv1.RevokeSessionRequest{
			AccessToken:  loginResp.AccessToken,
			ClientId:     "client-b",
			ClientSecret: testClientSecret,
			SessionId:    loginResp.SessionId,
		})
		if revokeResp.Success {
			t.Fatalf("expected RevokeSession via client-b to fail")
		}
	})
}

func TestRefreshToken_RotationAndReuse(t *testing.T) {
	db := newTestDB(t)
	svc := NewAuthServiceServer(db)
	seedClient(t, db, "client-1")
	seedUser(t, db, "user-1", "client-1", "alice@example.com", "alice", "password123")

	loginResp := login(t, svc, "alice@example.com", "password123", "client-1")

	refresh := func(token string) *authv1.RefreshTokenResponse {
		resp, err := svc.RefreshToken(context.Background(), &authv1.RefreshTokenRequest{
			RefreshToken: token,
			ClientId:     "client-1",
			ClientSecret: testClientSecret,
		})
		if err != nil {
			t.Fatalf("RefreshToken returned error: %v", err)
		}
		return resp
	}

	// Rotation: refresh succeeds and returns a different token
	first := refresh(loginResp.RefreshToken)
	if !first.Success || first.RefreshToken == "" || first.RefreshToken == loginResp.RefreshToken {
		t.Fatalf("expected rotated refresh token, got success=%v msg=%s", first.Success, first.Message)
	}

	// Reuse of the rotated (old) token: rejected AND the session is revoked
	reuse := refresh(loginResp.RefreshToken)
	if reuse.Success {
		t.Fatalf("expected reuse of rotated refresh token to fail")
	}
	repo := repository.NewAuthRepository(db)
	if _, err := repo.GetSessionByID(context.Background(), loginResp.SessionId); err == nil {
		t.Fatalf("expected session to be revoked after refresh token reuse")
	}

	// The newest token is dead too, since the whole session was revoked
	afterReuse := refresh(first.RefreshToken)
	if afterReuse.Success {
		t.Fatalf("expected current refresh token to be dead after session revocation")
	}
}

func TestLogoutUser_Success(t *testing.T) {
	db := newTestDB(t)
	svc := NewAuthServiceServer(db)
	seedClient(t, db, "client-1")
	seedUser(t, db, "user-1", "client-1", "alice@example.com", "alice", "password123")

	loginResp := login(t, svc, "alice@example.com", "password123", "client-1")

	resp, err := svc.RevokeToken(context.Background(), &authv1.RevokeTokenRequest{
		RefreshToken: loginResp.RefreshToken,
		ClientId:     "client-1",
		ClientSecret: testClientSecret,
	})
	if err != nil {
		t.Fatalf("RevokeToken returned error: %v", err)
	}
	if !resp.Success {
		t.Fatalf("expected success, got msg=%s", resp.Message)
	}

	// assert session removed
	repo := repository.NewAuthRepository(db)
	if _, err := repo.GetSessionByID(context.Background(), loginResp.SessionId); err == nil {
		t.Fatalf("expected session to be deleted")
	}
}

func TestSessionManagementRPCs(t *testing.T) {
	db := newTestDB(t)
	svc := NewAuthServiceServer(db)
	seedClient(t, db, "client-1")
	seedUser(t, db, "user-1", "client-1", "alice@example.com", "alice", "password123")

	first := login(t, svc, "alice@example.com", "password123", "client-1")
	second := login(t, svc, "alice@example.com", "password123", "client-1")

	// GetUserSessions lists both, marking the caller's session as current
	sessResp, err := svc.GetUserSessions(context.Background(), &authv1.GetUserSessionsRequest{
		AccessToken:  first.AccessToken,
		ClientId:     "client-1",
		ClientSecret: testClientSecret,
	})
	if err != nil || !sessResp.Success {
		t.Fatalf("GetUserSessions failed: %v msg=%s", err, sessResp.GetMessage())
	}
	if len(sessResp.Sessions) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(sessResp.Sessions))
	}
	currentCount := 0
	for _, s := range sessResp.Sessions {
		if s.Current {
			currentCount++
			if s.SessionId != first.SessionId {
				t.Fatalf("expected caller's session to be marked current")
			}
		}
	}
	if currentCount != 1 {
		t.Fatalf("expected exactly one current session, got %d", currentCount)
	}

	// RevokeSession kills the second session only
	revokeResp, err := svc.RevokeSession(context.Background(), &authv1.RevokeSessionRequest{
		AccessToken:  first.AccessToken,
		ClientId:     "client-1",
		ClientSecret: testClientSecret,
		SessionId:    second.SessionId,
	})
	if err != nil || !revokeResp.Success {
		t.Fatalf("RevokeSession failed: %v msg=%s", err, revokeResp.GetMessage())
	}
	check, _ := svc.ValidateToken(context.Background(), &authv1.ValidateTokenRequest{
		AccessToken:  second.AccessToken,
		ClientId:     "client-1",
		ClientSecret: testClientSecret,
	})
	if check.Valid {
		t.Fatalf("expected revoked session's token to be invalid")
	}

	// LogoutAllSessions kills the rest
	logoutResp, err := svc.LogoutAllSessions(context.Background(), &authv1.LogoutAllSessionsRequest{
		AccessToken:  first.AccessToken,
		ClientId:     "client-1",
		ClientSecret: testClientSecret,
	})
	if err != nil || !logoutResp.Success {
		t.Fatalf("LogoutAllSessions failed: %v msg=%s", err, logoutResp.GetMessage())
	}
	if logoutResp.RevokedCount != 1 {
		t.Fatalf("expected 1 remaining session revoked, got %d", logoutResp.RevokedCount)
	}
	check, _ = svc.ValidateToken(context.Background(), &authv1.ValidateTokenRequest{
		AccessToken:  first.AccessToken,
		ClientId:     "client-1",
		ClientSecret: testClientSecret,
	})
	if check.Valid {
		t.Fatalf("expected all tokens invalid after LogoutAllSessions")
	}
}

func TestChangePassword_KeepsCurrentSession(t *testing.T) {
	db := newTestDB(t)
	svc := NewAuthServiceServer(db)
	seedClient(t, db, "client-1")
	seedUser(t, db, "user-1", "client-1", "alice@example.com", "alice", "old-password")

	current := login(t, svc, "alice@example.com", "old-password", "client-1")
	other := login(t, svc, "alice@example.com", "old-password", "client-1")

	resp, err := svc.ChangeUserPassword(context.Background(), &authv1.ChangeUserPasswordRequest{
		AccessToken:     current.AccessToken,
		CurrentPassword: "old-password",
		NewPassword:     "new-password-123",
		ClientId:        "client-1",
		ClientSecret:    testClientSecret,
	})
	if err != nil {
		t.Fatalf("ChangeUserPassword returned error: %v", err)
	}
	if !resp.Success {
		t.Fatalf("expected success changing password, got msg=%s", resp.Message)
	}

	// Current session survives, other session is invalidated
	check, _ := svc.ValidateToken(context.Background(), &authv1.ValidateTokenRequest{
		AccessToken:  current.AccessToken,
		ClientId:     "client-1",
		ClientSecret: testClientSecret,
	})
	if !check.Valid {
		t.Fatalf("expected current session to survive password change")
	}
	check, _ = svc.ValidateToken(context.Background(), &authv1.ValidateTokenRequest{
		AccessToken:  other.AccessToken,
		ClientId:     "client-1",
		ClientSecret: testClientSecret,
	})
	if check.Valid {
		t.Fatalf("expected other session to be invalidated by password change")
	}

	// New password works, old one does not
	login(t, svc, "alice@example.com", "new-password-123", "client-1")
	failResp, _ := svc.GetToken(context.Background(), &authv1.GetTokenRequest{
		Email:        "alice@example.com",
		Password:     "old-password",
		ClientId:     "client-1",
		ClientSecret: testClientSecret,
	})
	if failResp.Success {
		t.Fatalf("expected old password to be rejected")
	}
}

func TestChangePassword_EnforcesPolicy(t *testing.T) {
	db := newTestDB(t)
	svc := NewAuthServiceServer(db)
	seedClient(t, db, "client-1")
	seedUser(t, db, "user-1", "client-1", "alice@example.com", "alice", "old-password")
	current := login(t, svc, "alice@example.com", "old-password", "client-1")

	resp, _ := svc.ChangeUserPassword(context.Background(), &authv1.ChangeUserPasswordRequest{
		AccessToken:     current.AccessToken,
		CurrentPassword: "old-password",
		NewPassword:     "short",
		ClientId:        "client-1",
		ClientSecret:    testClientSecret,
	})
	if resp.Success {
		t.Fatalf("expected <8-char new password to be rejected")
	}
}

func TestRegisterClient_AdminSecret(t *testing.T) {
	db := newTestDB(t)
	svc := NewAuthServiceServer(db)
	t.Setenv("ADMIN_SECRET", "super-admin")

	// wrong admin secret
	resp, _ := svc.RegisterClient(context.Background(), &authv1.RegisterClientRequest{
		ClientName:  "app",
		AdminSecret: "nope",
	})
	if resp.Success {
		t.Fatalf("expected RegisterClient with wrong admin secret to fail")
	}

	// correct admin secret
	resp, err := svc.RegisterClient(context.Background(), &authv1.RegisterClientRequest{
		ClientName:  "app",
		AdminSecret: "super-admin",
	})
	if err != nil || !resp.Success || resp.ClientId == "" || resp.ClientSecret == "" {
		t.Fatalf("expected RegisterClient to succeed, got msg=%s", resp.GetMessage())
	}

	// Secret must be stored hashed, not in plaintext
	repo := repository.NewAuthRepository(db)
	client, err := repo.GetClientByID(context.Background(), resp.ClientId)
	if err != nil {
		t.Fatalf("failed to load created client: %v", err)
	}
	if client.ClientSecretHash == resp.ClientSecret {
		t.Fatalf("client secret stored in plaintext")
	}
	if !utils.CheckPasswordHash(resp.ClientSecret, client.ClientSecretHash) {
		t.Fatalf("stored hash does not match issued secret")
	}
}

func TestChangeClientSecret_AdminSecret(t *testing.T) {
	db := newTestDB(t)
	svc := NewAuthServiceServer(db)
	seedClient(t, db, "client-1")
	t.Setenv("ADMIN_SECRET", "super-admin")

	// missing admin secret
	resp, _ := svc.ChangeClientSecret(context.Background(), &authv1.ChangeClientSecretRequest{
		ClientId:      "client-1",
		CurrentSecret: testClientSecret,
	})
	if resp.Success {
		t.Fatalf("expected ChangeClientSecret without admin secret to fail")
	}

	// with admin secret and correct current secret
	resp, err := svc.ChangeClientSecret(context.Background(), &authv1.ChangeClientSecretRequest{
		ClientId:      "client-1",
		CurrentSecret: testClientSecret,
		AdminSecret:   "super-admin",
	})
	if err != nil || !resp.Success || resp.ClientSecret == "" {
		t.Fatalf("expected ChangeClientSecret to succeed, got msg=%s", resp.GetMessage())
	}

	// old secret no longer authenticates
	if err := svc.authenticateClient(context.Background(), "client-1", testClientSecret); err == nil {
		t.Fatalf("expected old client secret to be rejected after rotation")
	}
	if err := svc.authenticateClient(context.Background(), "client-1", resp.ClientSecret); err != nil {
		t.Fatalf("expected new client secret to authenticate: %v", err)
	}
}
