package service

import (
	"context"
	"testing"

	authv1 "authservice/proto/auth/v1"
)

func ptrBool(b bool) *bool    { return &b }
func ptrStr(s string) *string { return &s }

// updateApp is a thin test helper over PlatformService.UpdateApp.
func updateApp(t *testing.T, f *hostedFixture, req *authv1.UpdateAppRequest) *authv1.UpdateAppResponse {
	t.Helper()
	req.AccessToken = f.devToken
	req.ClientId = f.clientID
	resp, err := f.plat.UpdateApp(context.Background(), req)
	if err != nil {
		t.Fatalf("UpdateApp returned error: %v", err)
	}
	return resp
}

// --- Feature 1: demo login -------------------------------------------------

func TestHostedDemoLogin(t *testing.T) {
	f := newHostedFixture(t)
	ctx := context.Background()

	demoReq := func() *authv1.HostedDemoLoginResponse {
		resp, err := f.plat.HostedDemoLogin(ctx, &authv1.HostedDemoLoginRequest{
			ClientId:    f.clientID,
			RedirectUri: hostedRedirect,
		})
		if err != nil {
			t.Fatalf("HostedDemoLogin error: %v", err)
		}
		return resp
	}

	// Disabled by default -> rejected.
	if r := demoReq(); r.Success {
		t.Fatalf("expected demo login to be unavailable when disabled")
	}

	// Enabling demo without credentials must fail.
	if r := updateApp(t, f, &authv1.UpdateAppRequest{DemoEnabled: ptrBool(true)}); r.Success {
		t.Fatalf("expected enabling demo without credentials to fail")
	}

	// Configure demo credentials pointing at the fixture user.
	if r := updateApp(t, f, &authv1.UpdateAppRequest{
		DemoEnabled:  ptrBool(true),
		DemoEmail:    ptrStr("user@example.com"),
		DemoPassword: ptrStr("userpass123"),
	}); !r.Success {
		t.Fatalf("expected demo config to succeed, got: %s", r.Message)
	}

	// Public info advertises demo, but never leaks credentials (no such fields).
	info, _ := f.plat.GetAppPublicInfo(ctx, &authv1.GetAppPublicInfoRequest{ClientId: f.clientID})
	if !info.DemoEnabled {
		t.Fatalf("expected GetAppPublicInfo.DemoEnabled=true")
	}

	// One-click demo login now works.
	r := demoReq()
	if !r.Success || r.AccessToken == "" || r.User == nil || r.User.Email != "user@example.com" {
		t.Fatalf("expected demo login to succeed with a token, got: %+v", r)
	}

	// Wrong redirect_uri is rejected.
	bad, _ := f.plat.HostedDemoLogin(ctx, &authv1.HostedDemoLoginRequest{ClientId: f.clientID, RedirectUri: "https://evil.example.com/x"})
	if bad.Success {
		t.Fatalf("expected demo login with unlisted redirect to fail")
	}
}

// --- Feature 2: login identifier modes -------------------------------------

func TestLoginIdentifierModes(t *testing.T) {
	f := newHostedFixture(t) // fixture user: username "user", email "user@example.com"
	ctx := context.Background()

	login := func(identifier string) bool {
		r, err := f.plat.HostedLogin(ctx, &authv1.HostedLoginRequest{
			ClientId: f.clientID, Email: identifier, Password: "userpass123", RedirectUri: hostedRedirect,
		})
		if err != nil {
			t.Fatalf("HostedLogin error: %v", err)
		}
		return r.Success
	}

	// Default (username_or_email): both work.
	if !login("user@example.com") || !login("user") {
		t.Fatalf("username_or_email should accept both email and username")
	}

	// email_only: email works, username does not.
	updateApp(t, f, &authv1.UpdateAppRequest{LoginIdentifier: ptrStr("email_only")})
	if !login("user@example.com") {
		t.Fatalf("email_only should accept email")
	}
	if login("user") {
		t.Fatalf("email_only should reject a bare username")
	}

	// username_only: username works, email does not.
	updateApp(t, f, &authv1.UpdateAppRequest{LoginIdentifier: ptrStr("username_only")})
	if !login("user") {
		t.Fatalf("username_only should accept username")
	}
	if login("user@example.com") {
		t.Fatalf("username_only should reject an email")
	}

	// Invalid mode rejected by UpdateApp.
	if r := updateApp(t, f, &authv1.UpdateAppRequest{LoginIdentifier: ptrStr("phone")}); r.Success {
		t.Fatalf("expected invalid login_identifier to be rejected")
	}
}

// --- Feature 3: public signup + invite-only --------------------------------

func TestHostedRegister(t *testing.T) {
	f := newHostedFixture(t)
	ctx := context.Background()

	register := func(username, email, password string) *authv1.HostedRegisterResponse {
		r, err := f.plat.HostedRegister(ctx, &authv1.HostedRegisterRequest{
			ClientId: f.clientID, Username: username, Email: email, Password: password, RedirectUri: hostedRedirect,
		})
		if err != nil {
			t.Fatalf("HostedRegister error: %v", err)
		}
		return r
	}

	// Invite-only by default -> rejected.
	if r := register("newbie", "newbie@example.com", "password123"); r.Success {
		t.Fatalf("expected signup to be unavailable when public_signup is off")
	}

	// Enable public signup.
	updateApp(t, f, &authv1.UpdateAppRequest{PublicSignup: ptrBool(true)})

	// Happy path: creates and auto-logs in.
	r := register("newbie", "newbie@example.com", "password123")
	if !r.Success || r.AccessToken == "" || r.User == nil || r.User.Email != "newbie@example.com" {
		t.Fatalf("expected signup to succeed with a token, got: %+v", r)
	}

	// Duplicate email and duplicate username both rejected.
	if register("other", "newbie@example.com", "password123").Success {
		t.Fatalf("expected duplicate email to be rejected")
	}
	if register("newbie", "other@example.com", "password123").Success {
		t.Fatalf("expected duplicate username to be rejected")
	}

	// Short password rejected.
	if register("shorty", "shorty@example.com", "short").Success {
		t.Fatalf("expected short password to be rejected")
	}
}

// --- Feature 4: self-edit + admin edit + field locks -----------------------

func TestSelfEditAndLocks(t *testing.T) {
	f := newHostedFixture(t)
	ctx := context.Background()
	uid := f.s1.User.UserId

	update := func(username, email *string) *authv1.HostedUpdateProfileResponse {
		r, err := f.plat.HostedUpdateProfile(ctx, &authv1.HostedUpdateProfileRequest{
			ClientId: f.clientID, AccessToken: f.s1.AccessToken, Username: username, Email: email,
		})
		if err != nil {
			t.Fatalf("HostedUpdateProfile error: %v", err)
		}
		return r
	}

	// Self-edit username + email works.
	if r := update(ptrStr("renamed"), ptrStr("renamed@example.com")); !r.Success || r.User.Username != "renamed" {
		t.Fatalf("expected self-edit to succeed, got: %+v", r)
	}

	// Admin locks username + password.
	if r := (func() *authv1.UpdateUserResponse {
		resp, err := f.plat.UpdateUser(ctx, &authv1.UpdateUserRequest{
			AccessToken: f.devToken, ClientId: f.clientID, UserId: uid,
			LockUsername: ptrBool(true), LockPassword: ptrBool(true),
		})
		if err != nil {
			t.Fatalf("UpdateUser error: %v", err)
		}
		return resp
	}()); !r.Success || !r.User.LockUsername || !r.User.LockPassword {
		t.Fatalf("expected admin lock to succeed, got: %+v", r)
	}

	// Self-edit of the locked username is now rejected; email still editable.
	if update(ptrStr("again"), nil).Success {
		t.Fatalf("expected locked username self-edit to be rejected")
	}
	if r := update(nil, ptrStr("changed@example.com")); !r.Success {
		t.Fatalf("expected unlocked email self-edit to succeed, got: %s", r.Message)
	}

	// Locked password blocks self password change.
	pw, _ := f.plat.HostedChangePassword(ctx, &authv1.HostedChangePasswordRequest{
		ClientId: f.clientID, AccessToken: f.s1.AccessToken, CurrentPassword: "userpass123", NewPassword: "newpass12345",
	})
	if pw.Success {
		t.Fatalf("expected locked password change to be rejected")
	}

	// Admin bypasses locks: can rename the locked username.
	adm, _ := f.plat.UpdateUser(ctx, &authv1.UpdateUserRequest{
		AccessToken: f.devToken, ClientId: f.clientID, UserId: uid, Username: ptrStr("adminset"),
	})
	if !adm.Success || adm.User.Username != "adminset" {
		t.Fatalf("expected admin to rename locked username, got: %+v", adm)
	}

	// Second user; self-edit to a taken username is rejected.
	registerNamedUser(t, f.auth, f.clientID, f.clientSecret, "taken", "taken@example.com", "password123")
	if update(ptrStr("taken"), nil).Success {
		t.Fatalf("expected rename to a taken username to be rejected")
	}
}
