package service

import (
	"context"
	"strings"
	"testing"

	"authservice/pkg/repository"
	authv1 "authservice/proto/auth/v1"
)

// setRedirectURIs updates an app's hosted-login whitelist via UpdateApp.
func setRedirectURIs(t *testing.T, plat *PlatformServiceServerImpl, token, clientID string, uris []string) *authv1.UpdateAppResponse {
	t.Helper()
	resp, err := plat.UpdateApp(context.Background(), &authv1.UpdateAppRequest{
		AccessToken:     token,
		ClientId:        clientID,
		RedirectUris:    uris,
		SetRedirectUris: true,
	})
	if err != nil {
		t.Fatalf("UpdateApp returned error: %v", err)
	}
	return resp
}

// registerEndUser creates an end user on an app via AuthService.RegisterUser.
func registerEndUser(t *testing.T, auth *AuthServiceServerImpl, clientID, clientSecret, email, password string) {
	t.Helper()
	// Username must be unique per scope; derive it from the email local part
	// so distinct callers don't collide.
	username := "enduser"
	if i := strings.IndexByte(email, '@'); i > 0 {
		username = email[:i]
	}
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
}

func hostedLogin(t *testing.T, plat *PlatformServiceServerImpl, clientID, email, password, redirectURI string) *authv1.HostedLoginResponse {
	t.Helper()
	resp, err := plat.HostedLogin(context.Background(), &authv1.HostedLoginRequest{
		ClientId:    clientID,
		Email:       email,
		Password:    password,
		RedirectUri: redirectURI,
	})
	if err != nil {
		t.Fatalf("HostedLogin returned error: %v", err)
	}
	return resp
}

func TestUpdateAppRedirectURIs(t *testing.T) {
	_, plat, _ := setupPlatform(t)
	registerDeveloper(t, plat, "dev@example.com", "password123", "acme")
	token := developerLogin(t, plat, "dev@example.com", "password123").AccessToken
	app := createApp(t, plat, token, "wordskali", "app").App

	// Default: no redirect URIs.
	if len(app.RedirectUris) != 0 {
		t.Fatalf("expected new app to have no redirect URIs, got %v", app.RedirectUris)
	}

	// Set a whitelist; it round-trips through the App message.
	resp := setRedirectURIs(t, plat, token, app.ClientId, []string{
		"https://wordskali.example.com/callback",
		"http://localhost:5173/callback",
	})
	if !resp.Success || len(resp.App.RedirectUris) != 2 {
		t.Fatalf("expected 2 redirect URIs saved, got success=%v uris=%v", resp.Success, resp.App.GetRedirectUris())
	}

	// An UpdateApp without the flag leaves the whitelist unchanged.
	name := "renamed"
	noTouch, err := plat.UpdateApp(context.Background(), &authv1.UpdateAppRequest{
		AccessToken: token,
		ClientId:    app.ClientId,
		Name:        &name,
	})
	if err != nil || !noTouch.Success || len(noTouch.App.RedirectUris) != 2 {
		t.Fatalf("expected redirect URIs unchanged without set_redirect_uris, got %+v err=%v", noTouch, err)
	}

	// Relative / malformed URIs are rejected.
	bad := setRedirectURIs(t, plat, token, app.ClientId, []string{"/callback"})
	if bad.Success {
		t.Fatalf("expected relative redirect URI to be rejected")
	}

	// Empty list with the flag clears the whitelist (hosted login disabled).
	cleared := setRedirectURIs(t, plat, token, app.ClientId, nil)
	if !cleared.Success || len(cleared.App.RedirectUris) != 0 {
		t.Fatalf("expected whitelist cleared, got %+v", cleared.App.GetRedirectUris())
	}
}

func TestGetAppPublicInfo(t *testing.T) {
	db, plat, _ := setupPlatform(t)
	registerDeveloper(t, plat, "dev@example.com", "password123", "acme")
	token := developerLogin(t, plat, "dev@example.com", "password123").AccessToken
	app := createApp(t, plat, token, "wordskali", "app").App

	ctx := context.Background()

	// Unknown client id leaks nothing: success=false only.
	unknown, err := plat.GetAppPublicInfo(ctx, &authv1.GetAppPublicInfoRequest{ClientId: "nope"})
	if err != nil {
		t.Fatalf("GetAppPublicInfo returned error: %v", err)
	}
	if unknown.Success || unknown.Name != "" || unknown.HostedLoginEnabled {
		t.Fatalf("expected empty success=false response for unknown id, got %+v", unknown)
	}

	// Known app, no whitelist: hosted login disabled.
	info, _ := plat.GetAppPublicInfo(ctx, &authv1.GetAppPublicInfoRequest{ClientId: app.ClientId})
	if !info.Success || info.Name != "wordskali" || info.HostedLoginEnabled {
		t.Fatalf("expected name with hosted login disabled, got %+v", info)
	}

	// Whitelist set: hosted login enabled.
	setRedirectURIs(t, plat, token, app.ClientId, []string{"https://wordskali.example.com/callback"})
	info, _ = plat.GetAppPublicInfo(ctx, &authv1.GetAppPublicInfoRequest{ClientId: app.ClientId})
	if !info.Success || !info.HostedLoginEnabled {
		t.Fatalf("expected hosted login enabled, got %+v", info)
	}

	// Suspended app: hosted login reported disabled.
	repo := repository.NewAuthRepository(db)
	if err := repo.SetClientSuspended(ctx, app.ClientId, true); err != nil {
		t.Fatalf("SetClientSuspended failed: %v", err)
	}
	info, _ = plat.GetAppPublicInfo(ctx, &authv1.GetAppPublicInfoRequest{ClientId: app.ClientId})
	if !info.Success || info.HostedLoginEnabled {
		t.Fatalf("expected hosted login disabled while suspended, got %+v", info)
	}
}

func TestHostedLogin(t *testing.T) {
	db, plat, auth := setupPlatform(t)
	registerDeveloper(t, plat, "dev@example.com", "password123", "acme")
	token := developerLogin(t, plat, "dev@example.com", "password123").AccessToken

	created := createApp(t, plat, token, "wordskali", "app")
	clientID, clientSecret := created.App.ClientId, created.ClientSecret
	registerEndUser(t, auth, clientID, clientSecret, "user@example.com", "userpass123")

	const goodRedirect = "https://wordskali.example.com/callback"
	setRedirectURIs(t, plat, token, clientID, []string{goodRedirect})

	ctx := context.Background()
	repo := repository.NewAuthRepository(db)

	// Happy path: whitelisted redirect, no client secret needed.
	ok := hostedLogin(t, plat, clientID, "user@example.com", "userpass123", goodRedirect)
	if !ok.Success {
		t.Fatalf("expected hosted login to succeed, got msg=%s", ok.Message)
	}
	if ok.AccessToken == "" || ok.RefreshToken == "" || ok.SessionId == "" || ok.ExpiresAt == nil {
		t.Fatalf("expected full token pair in response, got %+v", ok)
	}
	if ok.User == nil || ok.User.Email != "user@example.com" || ok.User.ClientId != clientID {
		t.Fatalf("unexpected user profile: %+v", ok.User)
	}
	if _, err := repo.GetSessionByID(ctx, ok.SessionId); err != nil {
		t.Fatalf("expected a live session row for hosted login: %v", err)
	}

	// Wrong password: generic failure.
	if r := hostedLogin(t, plat, clientID, "user@example.com", "wrongpass", goodRedirect); r.Success || r.AccessToken != "" {
		t.Fatalf("expected wrong password to fail without tokens")
	}

	// Wrong / unlisted / prefix-similar redirect URIs are all rejected —
	// matching is exact string equality, never prefix.
	for _, uri := range []string{
		"https://evil.example.com/callback",
		"https://wordskali.example.com/other",
		goodRedirect + "/extra",            // prefix-similar (longer)
		goodRedirect[:len(goodRedirect)-1], // prefix-similar (shorter)
		"",
	} {
		if r := hostedLogin(t, plat, clientID, "user@example.com", "userpass123", uri); r.Success {
			t.Fatalf("expected redirect_uri %q to be rejected", uri)
		}
	}

	// Unknown app rejected.
	if r := hostedLogin(t, plat, "no-such-app", "user@example.com", "userpass123", goodRedirect); r.Success {
		t.Fatalf("expected hosted login on unknown app to fail")
	}

	// Suspended app rejected even with a whitelisted redirect.
	if err := repo.SetClientSuspended(ctx, clientID, true); err != nil {
		t.Fatalf("SetClientSuspended failed: %v", err)
	}
	if r := hostedLogin(t, plat, clientID, "user@example.com", "userpass123", goodRedirect); r.Success {
		t.Fatalf("expected hosted login on suspended app to fail")
	}
	if err := repo.SetClientSuspended(ctx, clientID, false); err != nil {
		t.Fatalf("SetClientSuspended failed: %v", err)
	}

	// Empty whitelist = hosted login disabled.
	setRedirectURIs(t, plat, token, clientID, nil)
	if r := hostedLogin(t, plat, clientID, "user@example.com", "userpass123", goodRedirect); r.Success {
		t.Fatalf("expected hosted login to fail once the whitelist is cleared")
	}
}

func TestClientCredentialCheckAndListTuples(t *testing.T) {
	_, plat, _ := setupPlatform(t)
	registerDeveloper(t, plat, "dev@example.com", "password123", "acme")
	token := developerLogin(t, plat, "dev@example.com", "password123").AccessToken

	appA := createApp(t, plat, token, "app-a", "app")
	appB := createApp(t, plat, token, "app-b", "app")

	ctx := context.Background()

	// Seed one tuple in app A's scope (via the developer path).
	write, err := plat.WriteTuples(ctx, &authv1.WriteTuplesRequest{
		AccessToken: token,
		ClientId:    appA.App.ClientId,
		Tuples: []*authv1.Tuple{{
			ObjectType:  "board",
			ObjectId:    "daily",
			Relation:    "owner",
			SubjectType: "user",
			SubjectId:   "ada",
		}},
	})
	if err != nil || !write.Success {
		t.Fatalf("WriteTuples failed: err=%v msg=%s", err, write.GetMessage())
	}

	credCheck := func(clientID, clientSecret, subject, relation, objectType, objectID string) *authv1.CheckResponse {
		t.Helper()
		resp, err := plat.Check(ctx, &authv1.CheckRequest{
			ClientId:     clientID,
			ClientSecret: clientSecret,
			Subject:      subject,
			Relation:     relation,
			ObjectType:   objectType,
			ObjectId:     objectID,
		})
		if err != nil {
			t.Fatalf("Check returned error: %v", err)
		}
		return resp
	}

	// Allow: client A's own credentials, own scope, matching tuple.
	if r := credCheck(appA.App.ClientId, appA.ClientSecret, "user:ada", "owner", "board", "daily"); !r.Allowed {
		t.Fatalf("expected client-credential check to allow, got reason=%s", r.Reason)
	}
	// Deny: no matching tuple (default deny).
	if r := credCheck(appA.App.ClientId, appA.ClientSecret, "user:someone", "owner", "board", "daily"); r.Allowed {
		t.Fatalf("expected client-credential check to deny for unknown subject")
	}
	// Cannot escape scope: client B's id with client A's secret fails
	// authentication — the credential is the scope.
	if r := credCheck(appB.App.ClientId, appA.ClientSecret, "user:ada", "owner", "board", "daily"); r.Allowed {
		t.Fatalf("expected cross-client credential check to be denied")
	}
	// Wrong secret denied.
	if r := credCheck(appA.App.ClientId, "wrong-secret", "user:ada", "owner", "board", "daily"); r.Allowed {
		t.Fatalf("expected wrong client secret to be denied")
	}

	// ListTuples with client credentials returns the client's OWN tuples.
	list, err := plat.ListTuples(ctx, &authv1.ListTuplesRequest{
		ClientId:     appA.App.ClientId,
		ClientSecret: appA.ClientSecret,
	})
	if err != nil || !list.Success || len(list.Tuples) != 1 || list.Tuples[0].SubjectId != "ada" {
		t.Fatalf("expected client-credential ListTuples to return app A's tuple, got %+v err=%v", list, err)
	}

	// ...and cannot name a different client's scope.
	other, err := plat.ListTuples(ctx, &authv1.ListTuplesRequest{
		ClientId:     appB.App.ClientId,
		ClientSecret: appA.ClientSecret,
	})
	if err != nil || other.Success {
		t.Fatalf("expected cross-client ListTuples to fail, got %+v err=%v", other, err)
	}
}
