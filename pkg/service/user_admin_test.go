package service

import (
	"context"
	"strings"
	"testing"

	"authservice/pkg/repository"
	"authservice/pkg/utils"
	authv1 "authservice/proto/auth/v1"
)

func containsFold(s, substr string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}

// registerNamedUser creates an end user with a specific username (the shared
// registerEndUser helper derives the username from the email local part).
func registerNamedUser(t *testing.T, auth *AuthServiceServerImpl, clientID, clientSecret, username, email, password string) string {
	t.Helper()
	resp, err := auth.RegisterUser(context.Background(), &authv1.RegisterUserRequest{
		Username:     username,
		Email:        email,
		Password:     password,
		ClientId:     clientID,
		ClientSecret: clientSecret,
	})
	if err != nil || !resp.Success {
		t.Fatalf("RegisterUser failed: err=%v msg=%s", err, resp.GetMessage())
	}
	return resp.UserId
}

func listAppUsers(t *testing.T, plat *PlatformServiceServerImpl, token, clientID, query string) *authv1.ListAppUsersResponse {
	t.Helper()
	resp, err := plat.ListAppUsers(context.Background(), &authv1.ListAppUsersRequest{
		AccessToken: token,
		ClientId:    clientID,
		Query:       query,
	})
	if err != nil {
		t.Fatalf("ListAppUsers returned error: %v", err)
	}
	return resp
}

func setUserActive(t *testing.T, plat *PlatformServiceServerImpl, token, clientID, userID string, active bool) *authv1.SetUserActiveResponse {
	t.Helper()
	resp, err := plat.SetUserActive(context.Background(), &authv1.SetUserActiveRequest{
		AccessToken: token,
		ClientId:    clientID,
		UserId:      userID,
		Active:      active,
	})
	if err != nil {
		t.Fatalf("SetUserActive returned error: %v", err)
	}
	return resp
}

func findAppUser(users []*authv1.AppUser, userID string) *authv1.AppUser {
	for _, u := range users {
		if u.UserId == userID {
			return u
		}
	}
	return nil
}

func TestListAppUsers_ScopesQueryAndSessionCount(t *testing.T) {
	_, plat, auth := setupPlatform(t)
	registerDeveloper(t, plat, "dev@example.com", "password123", "acme")
	token := developerLogin(t, plat, "dev@example.com", "password123").AccessToken

	// App-scoped app with two users; alice holds two sessions.
	appResp := createApp(t, plat, token, "wordskali", "app")
	appID, appSecret := appResp.App.ClientId, appResp.ClientSecret
	aliceID := registerNamedUser(t, auth, appID, appSecret, "alice", "alice@example.com", "password123")
	bobID := registerNamedUser(t, auth, appID, appSecret, "bob", "bob@example.com", "password123")
	for range 2 {
		resp, err := auth.GetToken(context.Background(), &authv1.GetTokenRequest{
			Email: "alice@example.com", Password: "password123",
			ClientId: appID, ClientSecret: appSecret,
		})
		if err != nil || !resp.Success {
			t.Fatalf("GetToken failed: err=%v msg=%s", err, resp.GetMessage())
		}
	}

	list := listAppUsers(t, plat, token, appID, "")
	if !list.Success || len(list.Users) != 2 {
		t.Fatalf("expected 2 users, got success=%v n=%d msg=%s", list.Success, len(list.Users), list.Message)
	}
	alice, bob := findAppUser(list.Users, aliceID), findAppUser(list.Users, bobID)
	if alice == nil || bob == nil {
		t.Fatalf("expected alice and bob in the listing")
	}
	if alice.Scope != "app" || bob.Scope != "app" {
		t.Errorf("expected app scope labels, got %q/%q", alice.Scope, bob.Scope)
	}
	if !alice.Active || !bob.Active {
		t.Errorf("new users must default to active")
	}
	if alice.SessionCount != 2 || bob.SessionCount != 0 {
		t.Errorf("expected session counts 2/0, got %d/%d", alice.SessionCount, bob.SessionCount)
	}
	if alice.Username != "alice" || alice.Email != "alice@example.com" || alice.CreatedAt == nil {
		t.Errorf("alice row missing fields: %+v", alice)
	}

	// Query: substring on username OR email; LIKE metacharacters are literal.
	if got := listAppUsers(t, plat, token, appID, "ali").Users; len(got) != 1 || got[0].UserId != aliceID {
		t.Errorf("query 'ali' should match only alice, got %d rows", len(got))
	}
	if got := listAppUsers(t, plat, token, appID, "bob@example").Users; len(got) != 1 || got[0].UserId != bobID {
		t.Errorf("query on email should match only bob, got %d rows", len(got))
	}
	if got := listAppUsers(t, plat, token, appID, "%").Users; len(got) != 0 {
		t.Errorf("literal %% must not act as a wildcard; matched %d rows", len(got))
	}
	if got := listAppUsers(t, plat, token, appID, "_ob").Users; len(got) != 0 {
		t.Errorf("literal _ must not act as a wildcard; matched %d rows", len(got))
	}

	// Org-scoped app: users land in the org's shared pool, labeled "org";
	// app-scoped rows (predating a scope flip) are still listed as "app".
	orgAppResp := createApp(t, plat, token, "dsapanicle", "org")
	orgAppID, orgAppSecret := orgAppResp.App.ClientId, orgAppResp.ClientSecret
	carolID := registerNamedUser(t, auth, orgAppID, orgAppSecret, "carol", "carol@example.com", "password123")
	// Flip to app scope, register a direct app user, flip back.
	appScope, orgScope := "app", "org"
	if resp, err := plat.UpdateApp(context.Background(), &authv1.UpdateAppRequest{
		AccessToken: token, ClientId: orgAppID, IdentityScope: &appScope,
	}); err != nil || !resp.Success {
		t.Fatalf("UpdateApp failed: err=%v msg=%s", err, resp.GetMessage())
	}
	daveID := registerNamedUser(t, auth, orgAppID, orgAppSecret, "dave", "dave@example.com", "password123")
	if resp, err := plat.UpdateApp(context.Background(), &authv1.UpdateAppRequest{
		AccessToken: token, ClientId: orgAppID, IdentityScope: &orgScope,
	}); err != nil || !resp.Success {
		t.Fatalf("UpdateApp failed: err=%v msg=%s", err, resp.GetMessage())
	}

	orgList := listAppUsers(t, plat, token, orgAppID, "")
	if len(orgList.Users) != 2 {
		t.Fatalf("expected carol (org) + dave (app), got %d rows", len(orgList.Users))
	}
	if u := findAppUser(orgList.Users, carolID); u == nil || u.Scope != "org" {
		t.Errorf("carol should be listed with scope=org, got %+v", u)
	}
	if u := findAppUser(orgList.Users, daveID); u == nil || u.Scope != "app" {
		t.Errorf("dave should be listed with scope=app, got %+v", u)
	}
	// wordskali's users never leak into dsapanicle's listing.
	if findAppUser(orgList.Users, aliceID) != nil {
		t.Errorf("another app's users must not appear in the listing")
	}
}

func TestUserSessionsAdmin_ListAndRevoke(t *testing.T) {
	_, plat, auth := setupPlatform(t)
	registerDeveloper(t, plat, "dev@example.com", "password123", "acme")
	token := developerLogin(t, plat, "dev@example.com", "password123").AccessToken
	appResp := createApp(t, plat, token, "wordskali", "app")
	appID, appSecret := appResp.App.ClientId, appResp.ClientSecret
	userID := registerNamedUser(t, auth, appID, appSecret, "alice", "alice@example.com", "password123")

	var userToken string
	for range 2 {
		resp, err := auth.GetToken(context.Background(), &authv1.GetTokenRequest{
			Email: "alice@example.com", Password: "password123",
			ClientId: appID, ClientSecret: appSecret, UserAgent: "unit-test-agent",
		})
		if err != nil || !resp.Success {
			t.Fatalf("GetToken failed: err=%v msg=%s", err, resp.GetMessage())
		}
		userToken = resp.AccessToken
	}

	list, err := plat.ListUserSessionsAdmin(context.Background(), &authv1.ListUserSessionsAdminRequest{
		AccessToken: token, ClientId: appID, UserId: userID,
	})
	if err != nil || !list.Success {
		t.Fatalf("ListUserSessionsAdmin failed: err=%v msg=%s", err, list.GetMessage())
	}
	if len(list.Sessions) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(list.Sessions))
	}
	for _, s := range list.Sessions {
		if s.SessionId == "" || s.UserAgent != "unit-test-agent" || s.CreatedAt == nil || s.ExpiresAt == nil {
			t.Errorf("session row missing fields: %+v", s)
		}
		if s.Current {
			t.Errorf("admin listings must never mark a session as current")
		}
	}

	revoke, err := plat.RevokeUserSessionsAdmin(context.Background(), &authv1.RevokeUserSessionsAdminRequest{
		AccessToken: token, ClientId: appID, UserId: userID,
	})
	if err != nil || !revoke.Success {
		t.Fatalf("RevokeUserSessionsAdmin failed: err=%v msg=%s", err, revoke.GetMessage())
	}
	if revoke.RevokedCount != 2 {
		t.Errorf("expected 2 revoked sessions, got %d", revoke.RevokedCount)
	}

	list, err = plat.ListUserSessionsAdmin(context.Background(), &authv1.ListUserSessionsAdminRequest{
		AccessToken: token, ClientId: appID, UserId: userID,
	})
	if err != nil || !list.Success || len(list.Sessions) != 0 {
		t.Fatalf("expected no sessions after revoke, got %d (err=%v)", len(list.GetSessions()), err)
	}

	// Revocation-aware validation kills the outstanding token.
	valid, err := auth.ValidateToken(context.Background(), &authv1.ValidateTokenRequest{
		AccessToken: userToken, ClientId: appID, ClientSecret: appSecret,
	})
	if err != nil || valid.Valid {
		t.Fatalf("expected token to be invalid after admin revoke (err=%v)", err)
	}
}

func TestSetUserActive_DeactivateBlocksAllFlowsAndReactivateRestores(t *testing.T) {
	db, plat, auth := setupPlatform(t)
	registerDeveloper(t, plat, "dev@example.com", "password123", "acme")
	token := developerLogin(t, plat, "dev@example.com", "password123").AccessToken
	appResp := createApp(t, plat, token, "wordskali", "app")
	appID, appSecret := appResp.App.ClientId, appResp.ClientSecret
	setRedirectURIs(t, plat, token, appID, []string{"https://app.example.com/cb"})
	userID := registerNamedUser(t, auth, appID, appSecret, "alice", "alice@example.com", "password123")

	login := func() *authv1.GetTokenResponse {
		resp, err := auth.GetToken(context.Background(), &authv1.GetTokenRequest{
			Email: "alice@example.com", Password: "password123",
			ClientId: appID, ClientSecret: appSecret,
		})
		if err != nil {
			t.Fatalf("GetToken returned error: %v", err)
		}
		return resp
	}
	preToken := login().AccessToken

	deact := setUserActive(t, plat, token, appID, userID, false)
	if !deact.Success {
		t.Fatalf("SetUserActive(false) failed: %s", deact.Message)
	}
	if deact.User == nil || deact.User.Active {
		t.Errorf("response should reflect the deactivated state")
	}

	// GetToken: generic invalid credentials (no deactivation oracle).
	if resp := login(); resp.Success || resp.Message != "Invalid credentials" {
		t.Errorf("expected generic 'Invalid credentials', got success=%v msg=%q", resp.Success, resp.Message)
	}
	// HostedLogin: same shared path, same generic rejection.
	if resp := hostedLogin(t, plat, appID, "alice@example.com", "password123", "https://app.example.com/cb"); resp.Success || resp.Message != "Invalid credentials" {
		t.Errorf("expected hosted login to fail with 'Invalid credentials', got success=%v msg=%q", resp.Success, resp.Message)
	}
	// ValidateToken: the pre-deactivation token is dead.
	if resp, err := auth.ValidateToken(context.Background(), &authv1.ValidateTokenRequest{
		AccessToken: preToken, ClientId: appID, ClientSecret: appSecret,
	}); err != nil || resp.Valid {
		t.Errorf("expected pre-deactivation token to be invalid (err=%v)", err)
	}

	// Reactivate: login works again.
	react := setUserActive(t, plat, token, appID, userID, true)
	if !react.Success {
		t.Fatalf("SetUserActive(true) failed: %s", react.Message)
	}
	freshToken := login()
	if !freshToken.Success {
		t.Fatalf("expected login to succeed after reactivation, got %s", freshToken.Message)
	}

	// ValidateToken's own active check (independent of session revocation):
	// flip the flag directly so the session survives — the token must still
	// be rejected.
	repo := repository.NewAuthRepository(db)
	if err := repo.SetUserActive(context.Background(), userID, false); err != nil {
		t.Fatalf("SetUserActive repo call failed: %v", err)
	}
	if resp, err := auth.ValidateToken(context.Background(), &authv1.ValidateTokenRequest{
		AccessToken: freshToken.AccessToken, ClientId: appID, ClientSecret: appSecret,
	}); err != nil || resp.Valid {
		t.Errorf("expected ValidateToken to reject an inactive user's token even with a live session (err=%v)", err)
	}
}

func TestSetUserActive_OrgScopedRevokesSessionsAcrossClients(t *testing.T) {
	_, plat, auth := setupPlatform(t)
	registerDeveloper(t, plat, "dev@example.com", "password123", "acme")
	token := developerLogin(t, plat, "dev@example.com", "password123").AccessToken

	// Two org-scoped apps sharing the org's user pool.
	appA := createApp(t, plat, token, "app-a", "org")
	appB := createApp(t, plat, token, "app-b", "org")
	userID := registerNamedUser(t, auth, appA.App.ClientId, appA.ClientSecret, "olive", "olive@example.com", "password123")

	loginOn := func(clientID, secret string) string {
		resp, err := auth.GetToken(context.Background(), &authv1.GetTokenRequest{
			Email: "olive@example.com", Password: "password123",
			ClientId: clientID, ClientSecret: secret,
		})
		if err != nil || !resp.Success {
			t.Fatalf("GetToken failed on %s: err=%v msg=%s", clientID, err, resp.GetMessage())
		}
		return resp.AccessToken
	}
	tokenA := loginOn(appA.App.ClientId, appA.ClientSecret)
	tokenB := loginOn(appB.App.ClientId, appB.ClientSecret)

	// Deactivate through app A: the IDENTITY is disabled, so app B's session
	// dies too, and the message says so.
	resp := setUserActive(t, plat, token, appA.App.ClientId, userID, false)
	if !resp.Success {
		t.Fatalf("SetUserActive failed: %s", resp.Message)
	}
	if resp.User.GetScope() != "org" {
		t.Errorf("expected org scope label, got %q", resp.User.GetScope())
	}
	if msg := resp.Message; msg == "" || !containsFold(msg, "org") {
		t.Errorf("deactivating an org-scoped user must warn about the org-wide effect, got %q", msg)
	}
	for _, c := range []struct {
		clientID, secret, token string
	}{
		{appA.App.ClientId, appA.ClientSecret, tokenA},
		{appB.App.ClientId, appB.ClientSecret, tokenB},
	} {
		v, err := auth.ValidateToken(context.Background(), &authv1.ValidateTokenRequest{
			AccessToken: c.token, ClientId: c.clientID, ClientSecret: c.secret,
		})
		if err != nil || v.Valid {
			t.Errorf("expected token on %s to be revoked (err=%v)", c.clientID, err)
		}
	}
}

func TestUserAdmin_OwnershipIsolationAndSuperadminBypass(t *testing.T) {
	t.Setenv("SUPERADMIN_EMAILS", "root@example.com")
	_, plat, auth := setupPlatform(t)

	registerDeveloper(t, plat, "owner@example.com", "password123", "acme")
	ownerToken := developerLogin(t, plat, "owner@example.com", "password123").AccessToken
	appResp := createApp(t, plat, ownerToken, "wordskali", "app")
	appID, appSecret := appResp.App.ClientId, appResp.ClientSecret
	userID := registerNamedUser(t, auth, appID, appSecret, "alice", "alice@example.com", "password123")

	registerDeveloper(t, plat, "intruder@example.com", "password123", "evil")
	intruderToken := developerLogin(t, plat, "intruder@example.com", "password123").AccessToken

	// Every user-admin RPC refuses dev B on dev A's app.
	ctx := context.Background()
	if r, _ := plat.ListAppUsers(ctx, &authv1.ListAppUsersRequest{AccessToken: intruderToken, ClientId: appID}); r.Success {
		t.Errorf("ListAppUsers must not cross org boundaries")
	}
	if r, _ := plat.ListUserSessionsAdmin(ctx, &authv1.ListUserSessionsAdminRequest{AccessToken: intruderToken, ClientId: appID, UserId: userID}); r.Success {
		t.Errorf("ListUserSessionsAdmin must not cross org boundaries")
	}
	if r, _ := plat.RevokeUserSessionsAdmin(ctx, &authv1.RevokeUserSessionsAdminRequest{AccessToken: intruderToken, ClientId: appID, UserId: userID}); r.Success {
		t.Errorf("RevokeUserSessionsAdmin must not cross org boundaries")
	}
	if r, _ := plat.SetUserActive(ctx, &authv1.SetUserActiveRequest{AccessToken: intruderToken, ClientId: appID, UserId: userID, Active: false}); r.Success {
		t.Errorf("SetUserActive must not cross org boundaries")
	}
	// And the user is untouched.
	if u := findAppUser(listAppUsers(t, plat, ownerToken, appID, "").Users, userID); u == nil || !u.Active {
		t.Fatalf("intruder calls must not have deactivated the user")
	}

	// Superadmin bypasses ownership on all four.
	registerDeveloper(t, plat, "root@example.com", "password123", "platform-root")
	rootToken := developerLogin(t, plat, "root@example.com", "password123").AccessToken
	if r := listAppUsers(t, plat, rootToken, appID, ""); !r.Success || len(r.Users) != 1 {
		t.Errorf("superadmin should list any app's users, got success=%v n=%d", r.Success, len(r.Users))
	}
	if r, err := plat.ListUserSessionsAdmin(ctx, &authv1.ListUserSessionsAdminRequest{AccessToken: rootToken, ClientId: appID, UserId: userID}); err != nil || !r.Success {
		t.Errorf("superadmin ListUserSessionsAdmin failed: err=%v msg=%s", err, r.GetMessage())
	}
	if r, err := plat.RevokeUserSessionsAdmin(ctx, &authv1.RevokeUserSessionsAdminRequest{AccessToken: rootToken, ClientId: appID, UserId: userID}); err != nil || !r.Success {
		t.Errorf("superadmin RevokeUserSessionsAdmin failed: err=%v msg=%s", err, r.GetMessage())
	}
	if r := setUserActive(t, plat, rootToken, appID, userID, false); !r.Success {
		t.Errorf("superadmin SetUserActive failed: %s", r.Message)
	}
}

func TestUserAdmin_CrossScopeTargetRejected(t *testing.T) {
	_, plat, auth := setupPlatform(t)
	registerDeveloper(t, plat, "dev@example.com", "password123", "acme")
	token := developerLogin(t, plat, "dev@example.com", "password123").AccessToken

	// Same org, but the target user belongs to app B's scope, not app A's.
	appA := createApp(t, plat, token, "app-a", "app")
	appB := createApp(t, plat, token, "app-b", "app")
	userB := registerNamedUser(t, auth, appB.App.ClientId, appB.ClientSecret, "bea", "bea@example.com", "password123")

	ctx := context.Background()
	if r, _ := plat.ListUserSessionsAdmin(ctx, &authv1.ListUserSessionsAdminRequest{AccessToken: token, ClientId: appA.App.ClientId, UserId: userB}); r.Success {
		t.Errorf("ListUserSessionsAdmin must reject a user outside the app's scope")
	}
	if r, _ := plat.RevokeUserSessionsAdmin(ctx, &authv1.RevokeUserSessionsAdminRequest{AccessToken: token, ClientId: appA.App.ClientId, UserId: userB}); r.Success {
		t.Errorf("RevokeUserSessionsAdmin must reject a user outside the app's scope")
	}
	r, _ := plat.SetUserActive(ctx, &authv1.SetUserActiveRequest{AccessToken: token, ClientId: appA.App.ClientId, UserId: userB, Active: false})
	if r.Success {
		t.Errorf("SetUserActive must reject a user outside the app's scope")
	}
	if r.Message != "User not found" {
		t.Errorf("cross-scope rejection must not leak details, got %q", r.Message)
	}
	// The user still logs in fine on their own app.
	if resp, err := auth.GetToken(ctx, &authv1.GetTokenRequest{
		Email: "bea@example.com", Password: "password123",
		ClientId: appB.App.ClientId, ClientSecret: appB.ClientSecret,
	}); err != nil || !resp.Success {
		t.Fatalf("user should remain active on their own app: err=%v msg=%s", err, resp.GetMessage())
	}
}

// Rows written before the active column existed (raw insert without the
// column) must come back active — the schema default is the migration.
func TestActiveColumnDefaultsPreexistingRowsToActive(t *testing.T) {
	db, plat, auth := setupPlatform(t)
	registerDeveloper(t, plat, "dev@example.com", "password123", "acme")
	token := developerLogin(t, plat, "dev@example.com", "password123").AccessToken
	appResp := createApp(t, plat, token, "wordskali", "app")
	appID, appSecret := appResp.App.ClientId, appResp.ClientSecret

	hash, err := utils.HashPassword("legacy-password")
	if err != nil {
		t.Fatalf("failed to hash password: %v", err)
	}
	if err := db.Exec(
		`INSERT INTO users (user_id, user_name, email_id, password, scope_type, scope_id, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
		"legacy-user", "legacy", "legacy@example.com", hash, "app", appID,
	).Error; err != nil {
		t.Fatalf("raw insert failed: %v", err)
	}

	repo := repository.NewAuthRepository(db)
	user, err := repo.GetUserByID(context.Background(), "legacy-user")
	if err != nil {
		t.Fatalf("GetUserByID failed: %v", err)
	}
	if !user.Active {
		t.Fatalf("legacy rows (no active column at insert time) must default to active")
	}
	// And the legacy user can actually log in.
	if resp, err := auth.GetToken(context.Background(), &authv1.GetTokenRequest{
		Email: "legacy@example.com", Password: "legacy-password",
		ClientId: appID, ClientSecret: appSecret,
	}); err != nil || !resp.Success {
		t.Fatalf("legacy user login failed: err=%v msg=%s", err, resp.GetMessage())
	}
}
