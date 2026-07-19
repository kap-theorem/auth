package service

import (
	"context"
	"testing"

	"authservice/pkg/repository"
	authv1 "authservice/proto/auth/v1"
)

// hostedFixture bundles the standard hosted-account test setup: a developer,
// an app with a hosted whitelist, one registered end user, and two live
// hosted sessions for that user (s1 is the "current device").
type hostedFixture struct {
	plat         *PlatformServiceServerImpl
	auth         *AuthServiceServerImpl
	repo         *repository.AuthRepository
	devToken     string
	clientID     string
	clientSecret string
	s1, s2       *authv1.HostedLoginResponse
}

const hostedRedirect = "https://wordskali.example.com/callback"

func newHostedFixture(t *testing.T) *hostedFixture {
	t.Helper()
	db, plat, auth := setupPlatform(t)
	f := &hostedFixture{plat: plat, auth: auth, repo: repository.NewAuthRepository(db)}

	registerDeveloper(t, plat, "dev@example.com", "password123", "acme")
	f.devToken = developerLogin(t, plat, "dev@example.com", "password123").AccessToken

	created := createApp(t, plat, f.devToken, "wordskali", "app")
	f.clientID, f.clientSecret = created.App.ClientId, created.ClientSecret
	registerEndUser(t, auth, f.clientID, f.clientSecret, "user@example.com", "userpass123")

	setRedirectURIs(t, plat, f.devToken, f.clientID, []string{hostedRedirect})

	f.s1 = hostedLogin(t, plat, f.clientID, "user@example.com", "userpass123", hostedRedirect)
	f.s2 = hostedLogin(t, plat, f.clientID, "user@example.com", "userpass123", hostedRedirect)
	if !f.s1.Success || !f.s2.Success {
		t.Fatalf("fixture hosted logins failed: %s / %s", f.s1.Message, f.s2.Message)
	}
	return f
}

func (f *hostedFixture) profile(t *testing.T, clientID, token string) *authv1.HostedGetProfileResponse {
	t.Helper()
	resp, err := f.plat.HostedGetProfile(context.Background(), &authv1.HostedGetProfileRequest{
		ClientId:    clientID,
		AccessToken: token,
	})
	if err != nil {
		t.Fatalf("HostedGetProfile returned error: %v", err)
	}
	return resp
}

func TestHostedGetProfile(t *testing.T) {
	f := newHostedFixture(t)

	resp := f.profile(t, f.clientID, f.s1.AccessToken)
	if !resp.Success {
		t.Fatalf("expected profile success, got msg=%s", resp.Message)
	}
	if resp.User == nil || resp.User.Email != "user@example.com" || resp.User.Username != "enduser" {
		t.Fatalf("unexpected profile: %+v", resp.User)
	}
	if len(resp.Sessions) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(resp.Sessions))
	}
	for _, sess := range resp.Sessions {
		if sess.SessionId != f.s1.SessionId && sess.SessionId != f.s2.SessionId {
			t.Fatalf("unexpected session id %s", sess.SessionId)
		}
		if wantCurrent := sess.SessionId == f.s1.SessionId; sess.Current != wantCurrent {
			t.Fatalf("session %s: current=%v, want %v", sess.SessionId, sess.Current, wantCurrent)
		}
		if sess.CreatedAt == nil || sess.ExpiresAt == nil {
			t.Fatalf("expected timestamps on session %s", sess.SessionId)
		}
	}
}

func TestHostedAccountTokenAndAppGating(t *testing.T) {
	f := newHostedFixture(t)
	ctx := context.Background()

	// Garbage / empty / tampered tokens rejected.
	for _, token := range []string{"", "garbage", "eyJhbGciOiJSUzI1NiJ9.bogus.sig"} {
		if r := f.profile(t, f.clientID, token); r.Success {
			t.Fatalf("expected token %q to be rejected", token)
		}
	}

	// Cross-client token rejected: a token minted for app A cannot be used
	// against app B's client_id, even though it is otherwise valid.
	other := createApp(t, f.plat, f.devToken, "otherapp", "app")
	setRedirectURIs(t, f.plat, f.devToken, other.App.ClientId, []string{"https://other.example.com/cb"})
	if r := f.profile(t, other.App.ClientId, f.s1.AccessToken); r.Success {
		t.Fatalf("expected cross-client token to be rejected")
	}

	// Hosted-disabled app rejected (whitelist cleared), even with a token
	// that is otherwise still valid.
	setRedirectURIs(t, f.plat, f.devToken, f.clientID, nil)
	if r := f.profile(t, f.clientID, f.s1.AccessToken); r.Success {
		t.Fatalf("expected hosted-disabled app to be rejected")
	}
	setRedirectURIs(t, f.plat, f.devToken, f.clientID, []string{hostedRedirect})

	// Suspended app rejected.
	if err := f.repo.SetClientSuspended(ctx, f.clientID, true); err != nil {
		t.Fatalf("SetClientSuspended failed: %v", err)
	}
	if r := f.profile(t, f.clientID, f.s1.AccessToken); r.Success {
		t.Fatalf("expected suspended app to be rejected")
	}
	if err := f.repo.SetClientSuspended(ctx, f.clientID, false); err != nil {
		t.Fatalf("SetClientSuspended failed: %v", err)
	}

	// Revoked-session token rejected (revocation-aware validation): once the
	// session row is gone the JWT is dead even before its expiry.
	if err := f.repo.DeleteSessionByID(ctx, f.s1.SessionId); err != nil {
		t.Fatalf("DeleteSessionByID failed: %v", err)
	}
	if r := f.profile(t, f.clientID, f.s1.AccessToken); r.Success {
		t.Fatalf("expected revoked-session token to be rejected")
	}
}

func TestHostedChangePassword(t *testing.T) {
	f := newHostedFixture(t)
	ctx := context.Background()

	change := func(token, current, next string) *authv1.HostedChangePasswordResponse {
		t.Helper()
		resp, err := f.plat.HostedChangePassword(ctx, &authv1.HostedChangePasswordRequest{
			ClientId:        f.clientID,
			AccessToken:     token,
			CurrentPassword: current,
			NewPassword:     next,
		})
		if err != nil {
			t.Fatalf("HostedChangePassword returned error: %v", err)
		}
		return resp
	}

	// Wrong current password rejected; nothing is revoked.
	if r := change(f.s1.AccessToken, "wrongpass", "newpass12345"); r.Success {
		t.Fatalf("expected wrong current password to be rejected")
	}
	if _, err := f.repo.GetSessionByID(ctx, f.s2.SessionId); err != nil {
		t.Fatalf("expected other session to survive a failed change: %v", err)
	}

	// Too-short new password rejected (same >=8 policy as registration).
	if r := change(f.s1.AccessToken, "userpass123", "short"); r.Success {
		t.Fatalf("expected short password to be rejected")
	}

	// Happy path: password changes, the caller's session survives, every
	// OTHER session is killed.
	if r := change(f.s1.AccessToken, "userpass123", "newpass12345"); !r.Success {
		t.Fatalf("expected password change to succeed, got msg=%s", r.Message)
	}
	if _, err := f.repo.GetSessionByID(ctx, f.s1.SessionId); err != nil {
		t.Fatalf("expected caller's session to survive: %v", err)
	}
	if _, err := f.repo.GetSessionByID(ctx, f.s2.SessionId); err == nil {
		t.Fatalf("expected other session to be revoked")
	}

	// The new password works, the old one does not.
	if r := hostedLogin(t, f.plat, f.clientID, "user@example.com", "newpass12345", hostedRedirect); !r.Success {
		t.Fatalf("expected login with new password to succeed, got msg=%s", r.Message)
	}
	if r := hostedLogin(t, f.plat, f.clientID, "user@example.com", "userpass123", hostedRedirect); r.Success {
		t.Fatalf("expected login with old password to fail")
	}
}

func TestHostedRevokeSession(t *testing.T) {
	f := newHostedFixture(t)
	ctx := context.Background()

	revoke := func(token, sessionID string) *authv1.HostedRevokeSessionResponse {
		t.Helper()
		resp, err := f.plat.HostedRevokeSession(ctx, &authv1.HostedRevokeSessionRequest{
			ClientId:    f.clientID,
			AccessToken: token,
			SessionId:   sessionID,
		})
		if err != nil {
			t.Fatalf("HostedRevokeSession returned error: %v", err)
		}
		return resp
	}

	// Someone else's session id rejected: another user's session under the
	// SAME client is invisible to the caller ("Session not found").
	registerEndUser(t, f.auth, f.clientID, f.clientSecret, "other@example.com", "otherpass123")
	otherLogin := hostedLogin(t, f.plat, f.clientID, "other@example.com", "otherpass123", hostedRedirect)
	if !otherLogin.Success {
		t.Fatalf("second user's login failed: %s", otherLogin.Message)
	}
	if r := revoke(f.s1.AccessToken, otherLogin.SessionId); r.Success {
		t.Fatalf("expected someone else's session id to be rejected")
	}
	if _, err := f.repo.GetSessionByID(ctx, otherLogin.SessionId); err != nil {
		t.Fatalf("expected the other user's session to survive: %v", err)
	}

	// Unknown session id rejected.
	if r := revoke(f.s1.AccessToken, "no-such-session"); r.Success {
		t.Fatalf("expected unknown session id to be rejected")
	}

	// Happy path: revoke the caller's other device; the current session and
	// the other user's session both survive.
	if r := revoke(f.s1.AccessToken, f.s2.SessionId); !r.Success {
		t.Fatalf("expected revoke to succeed, got msg=%s", r.Message)
	}
	if _, err := f.repo.GetSessionByID(ctx, f.s2.SessionId); err == nil {
		t.Fatalf("expected revoked session to be gone")
	}
	if _, err := f.repo.GetSessionByID(ctx, f.s1.SessionId); err != nil {
		t.Fatalf("expected caller's session to survive: %v", err)
	}
}

func TestHostedLogoutAll(t *testing.T) {
	f := newHostedFixture(t)
	ctx := context.Background()

	// A second user's session must NOT be touched by the first user's
	// logout-all.
	registerEndUser(t, f.auth, f.clientID, f.clientSecret, "other@example.com", "otherpass123")
	otherLogin := hostedLogin(t, f.plat, f.clientID, "other@example.com", "otherpass123", hostedRedirect)

	resp, err := f.plat.HostedLogoutAll(ctx, &authv1.HostedLogoutAllRequest{
		ClientId:    f.clientID,
		AccessToken: f.s1.AccessToken,
	})
	if err != nil || !resp.Success {
		t.Fatalf("HostedLogoutAll failed: err=%v msg=%s", err, resp.GetMessage())
	}

	// EVERY session of the caller is gone, including the current one.
	for _, id := range []string{f.s1.SessionId, f.s2.SessionId} {
		if _, err := f.repo.GetSessionByID(ctx, id); err == nil {
			t.Fatalf("expected session %s to be revoked", id)
		}
	}

	// The presented token died with its session.
	if r := f.profile(t, f.clientID, f.s1.AccessToken); r.Success {
		t.Fatalf("expected the caller's token to be dead after logout-all")
	}

	// The other user is untouched.
	if _, err := f.repo.GetSessionByID(ctx, otherLogin.SessionId); err != nil {
		t.Fatalf("expected the other user's session to survive: %v", err)
	}
}
