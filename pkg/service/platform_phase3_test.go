package service

import (
	"context"
	"testing"

	"authservice/pkg/models"
	authv1 "authservice/proto/auth/v1"
)

// TestFullCheckResolution exercises the Phase 3 resolver end-to-end through
// the Check RPC: model implications, userset hops, deny-wins, and ABAC
// conditions with request context. (The exhaustive resolver table lives in
// pkg/authz/resolver_test.go.)
func TestFullCheckResolution(t *testing.T) {
	_, plat, _ := setupPlatform(t)
	registerDeveloper(t, plat, "alice@example.com", "password123", "")
	alice := developerLogin(t, plat, "alice@example.com", "password123")
	app := createApp(t, plat, alice.AccessToken, "app", "")
	appID := app.App.ClientId

	model, _ := plat.WriteAuthzModel(context.Background(), &authv1.WriteAuthzModelRequest{
		AccessToken: alice.AccessToken, ClientId: appID,
		ModelJson: `{"types": {"problem": {"relations": {"author": [], "editor": ["author"], "viewer": ["editor"]}}}}`,
	})
	if !model.Success {
		t.Fatalf("WriteAuthzModel failed: %s", model.Message)
	}

	write, _ := plat.WriteTuples(context.Background(), &authv1.WriteTuplesRequest{
		AccessToken: alice.AccessToken, ClientId: appID,
		Tuples: []*authv1.Tuple{
			{ObjectType: "problem", ObjectId: "two-sum", Relation: "author", SubjectType: "user", SubjectId: "kush"},
			{ObjectType: "problem", ObjectId: "two-sum", Relation: "editor", SubjectType: "role", SubjectId: "moderator"},
			{ObjectType: "role", ObjectId: "moderator", Relation: "member", SubjectType: "user", SubjectId: "mina"},
			{ObjectType: "problem", ObjectId: "two-sum", Relation: "viewer", SubjectType: "user", SubjectId: "banned", Effect: models.EffectDeny},
			{ObjectType: "problem", ObjectId: "two-sum", Relation: "viewer", SubjectType: "user", SubjectId: "banned"},
			{ObjectType: "problem", ObjectId: "staged", Relation: "viewer", SubjectType: "user", SubjectId: "contractor", ConditionExpr: `env == "staging"`},
		},
	})
	if !write.Success {
		t.Fatalf("WriteTuples failed: %s", write.Message)
	}

	check := func(subject, relation, objectID string, ctx map[string]string) *authv1.CheckResponse {
		t.Helper()
		resp, err := plat.Check(context.Background(), &authv1.CheckRequest{
			AccessToken: alice.AccessToken, ClientId: appID,
			Subject: subject, Relation: relation, ObjectType: "problem", ObjectId: objectID,
			Context: ctx,
		})
		if err != nil {
			t.Fatalf("Check returned error: %v", err)
		}
		return resp
	}

	if resp := check("user:kush", "viewer", "two-sum", nil); !resp.Allowed {
		t.Fatalf("expected implication chain allow (author ⇒ viewer), got deny (%s)", resp.Reason)
	}
	if resp := check("user:mina", "viewer", "two-sum", nil); !resp.Allowed {
		t.Fatalf("expected userset + implication allow, got deny (%s)", resp.Reason)
	}
	if resp := check("user:mina", "author", "two-sum", nil); resp.Allowed {
		t.Fatalf("expected editor role not to grant author (%s)", resp.Reason)
	}
	if resp := check("user:banned", "viewer", "two-sum", nil); resp.Allowed {
		t.Fatalf("expected explicit deny to win over allow (%s)", resp.Reason)
	}
	if resp := check("user:contractor", "viewer", "staged", map[string]string{"env": "staging"}); !resp.Allowed {
		t.Fatalf("expected conditioned allow with matching context, got deny (%s)", resp.Reason)
	}
	if resp := check("user:contractor", "viewer", "staged", map[string]string{"env": "prod"}); resp.Allowed {
		t.Fatalf("expected conditioned allow to be ignored with non-matching context (%s)", resp.Reason)
	}

	// ListObjects: reverse expansion, consistent with Check (empty context)
	list, err := plat.ListObjects(context.Background(), &authv1.ListObjectsRequest{
		AccessToken: alice.AccessToken, ClientId: appID,
		Subject: "user:mina", Relation: "viewer", ObjectType: "problem",
	})
	if err != nil {
		t.Fatalf("ListObjects returned error: %v", err)
	}
	if !list.Success || len(list.ObjectIds) != 1 || list.ObjectIds[0] != "two-sum" {
		t.Fatalf("expected ListObjects(user:mina, viewer, problem) = [two-sum], got %+v", list)
	}
	// Conditioned allow excluded under the empty context
	list, _ = plat.ListObjects(context.Background(), &authv1.ListObjectsRequest{
		AccessToken: alice.AccessToken, ClientId: appID,
		Subject: "user:contractor", Relation: "viewer", ObjectType: "problem",
	})
	if !list.Success || len(list.ObjectIds) != 0 {
		t.Fatalf("expected conditioned allow to be excluded from ListObjects, got %+v", list)
	}

	// Ownership gate: another developer cannot query this app's objects
	registerDeveloper(t, plat, "bob@example.com", "password123", "")
	bob := developerLogin(t, plat, "bob@example.com", "password123")
	list, _ = plat.ListObjects(context.Background(), &authv1.ListObjectsRequest{
		AccessToken: bob.AccessToken, ClientId: appID,
		Subject: "user:mina", Relation: "viewer", ObjectType: "problem",
	})
	if list.Success {
		t.Fatalf("expected ListObjects to be rejected for a non-owning developer")
	}
}

// TestDeveloperRefreshToken verifies the browser-facing refresh flow:
// rotation on every refresh, and reuse of a rotated token revoking the
// whole session (same semantics as AuthService.RefreshToken).
func TestDeveloperRefreshToken(t *testing.T) {
	_, plat, _ := setupPlatform(t)
	registerDeveloper(t, plat, "alice@example.com", "password123", "")
	alice := developerLogin(t, plat, "alice@example.com", "password123")

	// Refresh succeeds and rotates both tokens
	first, err := plat.DeveloperRefreshToken(context.Background(), &authv1.DeveloperRefreshTokenRequest{
		RefreshToken: alice.RefreshToken,
	})
	if err != nil {
		t.Fatalf("DeveloperRefreshToken returned error: %v", err)
	}
	if !first.Success || first.AccessToken == "" || first.RefreshToken == "" {
		t.Fatalf("expected successful refresh, got %+v", first)
	}
	if first.RefreshToken == alice.RefreshToken {
		t.Fatalf("expected the refresh token to rotate")
	}

	// The new access token authenticates platform RPCs
	apps, _ := plat.ListApps(context.Background(), &authv1.ListAppsRequest{AccessToken: first.AccessToken})
	if !apps.Success {
		t.Fatalf("expected refreshed access token to authenticate, got %s", apps.Message)
	}

	// Replaying the rotated (stale) token is a theft signal → session revoked
	replay, _ := plat.DeveloperRefreshToken(context.Background(), &authv1.DeveloperRefreshTokenRequest{
		RefreshToken: alice.RefreshToken,
	})
	if replay.Success {
		t.Fatalf("expected replay of a rotated refresh token to fail")
	}
	// ... which also kills the current refresh token and the session
	after, _ := plat.DeveloperRefreshToken(context.Background(), &authv1.DeveloperRefreshTokenRequest{
		RefreshToken: first.RefreshToken,
	})
	if after.Success {
		t.Fatalf("expected the whole session to be revoked after reuse detection")
	}
	apps, _ = plat.ListApps(context.Background(), &authv1.ListAppsRequest{AccessToken: first.AccessToken})
	if apps.Success {
		t.Fatalf("expected access token to be rejected after session revocation")
	}

	// Garbage and empty tokens fail
	if resp, _ := plat.DeveloperRefreshToken(context.Background(), &authv1.DeveloperRefreshTokenRequest{RefreshToken: "not-a-token"}); resp.Success {
		t.Fatalf("expected malformed refresh token to fail")
	}
	if resp, _ := plat.DeveloperRefreshToken(context.Background(), &authv1.DeveloperRefreshTokenRequest{}); resp.Success {
		t.Fatalf("expected empty refresh token to fail")
	}
}
