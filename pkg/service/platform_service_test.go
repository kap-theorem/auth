package service

import (
	"context"
	"testing"

	"authservice/pkg/models"
	"authservice/pkg/repository"
	authv1 "authservice/proto/auth/v1"
	"authservice/pkg/utils"

	"gorm.io/gorm"
)

const testPlatformSecret = "platform-secret"

// setupPlatform builds an in-memory DB with the platform org/client
// bootstrapped, plus both service implementations.
func setupPlatform(t *testing.T) (*gorm.DB, *PlatformServiceServerImpl, *AuthServiceServerImpl) {
	t.Helper()
	t.Setenv("PLATFORM_CLIENT_SECRET", testPlatformSecret)
	db := newTestDB(t)
	if err := BootstrapPlatform(context.Background(), db); err != nil {
		t.Fatalf("BootstrapPlatform failed: %v", err)
	}
	return db, NewPlatformServiceServer(db), NewAuthServiceServer(db)
}

func registerDeveloper(t *testing.T, svc *PlatformServiceServerImpl, email, password, orgName string) *authv1.RegisterDeveloperResponse {
	t.Helper()
	resp, err := svc.RegisterDeveloper(context.Background(), &authv1.RegisterDeveloperRequest{
		Email:    email,
		Password: password,
		OrgName:  orgName,
	})
	if err != nil {
		t.Fatalf("RegisterDeveloper returned error: %v", err)
	}
	if !resp.Success {
		t.Fatalf("expected developer registration to succeed, got msg=%s", resp.Message)
	}
	return resp
}

func developerLogin(t *testing.T, svc *PlatformServiceServerImpl, email, password string) *authv1.DeveloperLoginResponse {
	t.Helper()
	resp, err := svc.DeveloperLogin(context.Background(), &authv1.DeveloperLoginRequest{
		Email:    email,
		Password: password,
	})
	if err != nil {
		t.Fatalf("DeveloperLogin returned error: %v", err)
	}
	if !resp.Success {
		t.Fatalf("expected developer login to succeed, got msg=%s", resp.Message)
	}
	return resp
}

func createApp(t *testing.T, svc *PlatformServiceServerImpl, token, name, identityScope string) *authv1.CreateAppResponse {
	t.Helper()
	resp, err := svc.CreateApp(context.Background(), &authv1.CreateAppRequest{
		AccessToken:   token,
		Name:          name,
		IdentityScope: identityScope,
	})
	if err != nil {
		t.Fatalf("CreateApp returned error: %v", err)
	}
	if !resp.Success {
		t.Fatalf("expected app creation to succeed, got msg=%s", resp.Message)
	}
	return resp
}

func TestRegisterDeveloperAndLogin(t *testing.T) {
	db, plat, _ := setupPlatform(t)

	reg := registerDeveloper(t, plat, "dev@example.com", "password123", "acme")
	if reg.DeveloperId == "" || reg.OrgId == "" {
		t.Fatalf("expected developer_id and org_id in response")
	}

	// Personal org auto-created with the requested name
	repo := repository.NewAuthRepository(db)
	org, err := repo.GetOrganizationByID(context.Background(), reg.OrgId)
	if err != nil || org.Name != "acme" {
		t.Fatalf("expected personal org 'acme' to exist, got %+v err=%v", org, err)
	}

	// Duplicate email rejected
	dup, _ := plat.RegisterDeveloper(context.Background(), &authv1.RegisterDeveloperRequest{
		Email:    "dev@example.com",
		Password: "password123",
	})
	if dup.Success {
		t.Fatalf("expected duplicate developer registration to fail")
	}

	// Short password rejected
	short, _ := plat.RegisterDeveloper(context.Background(), &authv1.RegisterDeveloperRequest{
		Email:    "dev2@example.com",
		Password: "short",
	})
	if short.Success {
		t.Fatalf("expected short password to be rejected")
	}

	login := developerLogin(t, plat, "dev@example.com", "password123")
	if login.AccessToken == "" || login.RefreshToken == "" {
		t.Fatalf("expected tokens in login response")
	}
	if login.Developer == nil || login.Developer.DeveloperId != reg.DeveloperId ||
		login.Developer.OrgId != reg.OrgId || login.Developer.OrgName != "acme" {
		t.Fatalf("unexpected developer profile: %+v", login.Developer)
	}
	if login.IsSuperadmin {
		t.Fatalf("expected fresh developer not to be superadmin")
	}

	// Wrong password fails with a generic message
	bad, _ := plat.DeveloperLogin(context.Background(), &authv1.DeveloperLoginRequest{
		Email:    "dev@example.com",
		Password: "wrong-password",
	})
	if bad.Success {
		t.Fatalf("expected wrong password to fail")
	}
}

func TestAppCRUDAndOwnershipIsolation(t *testing.T) {
	_, plat, _ := setupPlatform(t)

	registerDeveloper(t, plat, "alice@example.com", "password123", "alice-org")
	registerDeveloper(t, plat, "bob@example.com", "password123", "bob-org")
	alice := developerLogin(t, plat, "alice@example.com", "password123")
	bob := developerLogin(t, plat, "bob@example.com", "password123")

	created := createApp(t, plat, alice.AccessToken, "wordskali", "")
	app := created.App
	if app.IdentityScope != models.ScopeApp {
		t.Fatalf("expected default identity_scope 'app', got %s", app.IdentityScope)
	}
	if app.OrgId != alice.Developer.OrgId {
		t.Fatalf("expected app to belong to alice's org")
	}

	// ListApps: alice sees her app, bob sees none
	aliceApps, _ := plat.ListApps(context.Background(), &authv1.ListAppsRequest{AccessToken: alice.AccessToken})
	if !aliceApps.Success || len(aliceApps.Apps) != 1 || aliceApps.Apps[0].ClientId != app.ClientId {
		t.Fatalf("expected alice to list exactly her app, got %+v", aliceApps)
	}
	bobApps, _ := plat.ListApps(context.Background(), &authv1.ListAppsRequest{AccessToken: bob.AccessToken})
	if !bobApps.Success || len(bobApps.Apps) != 0 {
		t.Fatalf("expected bob to list no apps, got %+v", bobApps)
	}

	// Bob cannot touch alice's app
	newName := "stolen"
	if resp, _ := plat.UpdateApp(context.Background(), &authv1.UpdateAppRequest{
		AccessToken: bob.AccessToken, ClientId: app.ClientId, Name: &newName,
	}); resp.Success {
		t.Fatalf("expected bob's UpdateApp on alice's app to fail")
	}
	if resp, _ := plat.RotateAppSecret(context.Background(), &authv1.RotateAppSecretRequest{
		AccessToken: bob.AccessToken, ClientId: app.ClientId,
	}); resp.Success {
		t.Fatalf("expected bob's RotateAppSecret on alice's app to fail")
	}
	if resp, _ := plat.DeleteApp(context.Background(), &authv1.DeleteAppRequest{
		AccessToken: bob.AccessToken, ClientId: app.ClientId,
	}); resp.Success {
		t.Fatalf("expected bob's DeleteApp on alice's app to fail")
	}
	if resp, _ := plat.WriteTuples(context.Background(), &authv1.WriteTuplesRequest{
		AccessToken: bob.AccessToken, ClientId: app.ClientId,
		Tuples: []*authv1.Tuple{{ObjectType: "doc", ObjectId: "1", Relation: "viewer", SubjectType: "user", SubjectId: "u1"}},
	}); resp.Success {
		t.Fatalf("expected bob's WriteTuples on alice's app to fail")
	}
	if resp, _ := plat.ListTuples(context.Background(), &authv1.ListTuplesRequest{
		AccessToken: bob.AccessToken, ClientId: app.ClientId,
	}); resp.Success {
		t.Fatalf("expected bob's ListTuples on alice's app to fail")
	}

	// Alice can update her app
	rename := "wordskali-2"
	orgScope := models.ScopeOrg
	updated, _ := plat.UpdateApp(context.Background(), &authv1.UpdateAppRequest{
		AccessToken:   alice.AccessToken,
		ClientId:      app.ClientId,
		Name:          &rename,
		IdentityScope: &orgScope,
	})
	if !updated.Success || updated.App.Name != "wordskali-2" || updated.App.IdentityScope != models.ScopeOrg {
		t.Fatalf("expected alice's UpdateApp to succeed, got %+v", updated)
	}

	// Alice can delete her app; it disappears from listings
	deleted, _ := plat.DeleteApp(context.Background(), &authv1.DeleteAppRequest{
		AccessToken: alice.AccessToken, ClientId: app.ClientId,
	})
	if !deleted.Success {
		t.Fatalf("expected alice's DeleteApp to succeed, got msg=%s", deleted.Message)
	}
	aliceApps, _ = plat.ListApps(context.Background(), &authv1.ListAppsRequest{AccessToken: alice.AccessToken})
	if len(aliceApps.Apps) != 0 {
		t.Fatalf("expected deleted app to disappear from ListApps")
	}
}

func TestCreateAndRotateAppSecret_ShowOnceHashAtRest(t *testing.T) {
	db, plat, auth := setupPlatform(t)
	registerDeveloper(t, plat, "alice@example.com", "password123", "")
	alice := developerLogin(t, plat, "alice@example.com", "password123")

	created := createApp(t, plat, alice.AccessToken, "app", "")
	repo := repository.NewAuthRepository(db)

	// Creation secret stored only as a bcrypt hash
	client, err := repo.GetClientByID(context.Background(), created.App.ClientId)
	if err != nil {
		t.Fatalf("failed to load created app: %v", err)
	}
	if client.ClientSecretHash == created.ClientSecret {
		t.Fatalf("client secret stored in plaintext")
	}
	if !utils.CheckPasswordHash(created.ClientSecret, client.ClientSecretHash) {
		t.Fatalf("stored hash does not match issued secret")
	}

	// Rotate: new secret returned once, hash updated, old secret dead
	rotated, _ := plat.RotateAppSecret(context.Background(), &authv1.RotateAppSecretRequest{
		AccessToken: alice.AccessToken, ClientId: created.App.ClientId,
	})
	if !rotated.Success || rotated.ClientSecret == "" || rotated.ClientSecret == created.ClientSecret {
		t.Fatalf("expected a fresh rotated secret, got %+v", rotated)
	}
	client, _ = repo.GetClientByID(context.Background(), created.App.ClientId)
	if client.ClientSecretHash == rotated.ClientSecret {
		t.Fatalf("rotated secret stored in plaintext")
	}
	if !utils.CheckPasswordHash(rotated.ClientSecret, client.ClientSecretHash) {
		t.Fatalf("stored hash does not match rotated secret")
	}
	if _, err := auth.authenticateClient(context.Background(), created.App.ClientId, created.ClientSecret); err == nil {
		t.Fatalf("expected old app secret to be rejected after rotation")
	}
	if _, err := auth.authenticateClient(context.Background(), created.App.ClientId, rotated.ClientSecret); err != nil {
		t.Fatalf("expected new app secret to authenticate: %v", err)
	}
}

func TestSuspendClientBlocksAppAuth(t *testing.T) {
	t.Setenv("SUPERADMIN_EMAILS", "root@example.com")
	_, plat, auth := setupPlatform(t)

	registerDeveloper(t, plat, "root@example.com", "password123", "")
	root := developerLogin(t, plat, "root@example.com", "password123")
	if !root.IsSuperadmin {
		t.Fatalf("expected root to be superadmin")
	}
	registerDeveloper(t, plat, "alice@example.com", "password123", "")
	alice := developerLogin(t, plat, "alice@example.com", "password123")

	created := createApp(t, plat, alice.AccessToken, "app", "")
	appID, appSecret := created.App.ClientId, created.ClientSecret

	// Register+login works pre-suspension
	reg, _ := auth.RegisterUser(context.Background(), &authv1.RegisterUserRequest{
		Username: "u1", Email: "u1@example.com", Password: "password123",
		ClientId: appID, ClientSecret: appSecret,
	})
	if !reg.Success {
		t.Fatalf("expected pre-suspension registration to succeed: %s", reg.Message)
	}
	loginResp, _ := auth.GetToken(context.Background(), &authv1.GetTokenRequest{
		Email: "u1@example.com", Password: "password123",
		ClientId: appID, ClientSecret: appSecret,
	})
	if !loginResp.Success {
		t.Fatalf("expected pre-suspension login to succeed: %s", loginResp.Message)
	}

	// Non-superadmin cannot suspend
	if resp, _ := plat.SuspendClient(context.Background(), &authv1.SuspendClientRequest{
		AccessToken: alice.AccessToken, ClientId: appID,
	}); resp.Success {
		t.Fatalf("expected non-superadmin SuspendClient to fail")
	}

	// Superadmin suspends; all app RPC auth fails
	susp, _ := plat.SuspendClient(context.Background(), &authv1.SuspendClientRequest{
		AccessToken: root.AccessToken, ClientId: appID,
	})
	if !susp.Success {
		t.Fatalf("expected superadmin SuspendClient to succeed: %s", susp.Message)
	}
	if resp, _ := auth.GetToken(context.Background(), &authv1.GetTokenRequest{
		Email: "u1@example.com", Password: "password123",
		ClientId: appID, ClientSecret: appSecret,
	}); resp.Success {
		t.Fatalf("expected login to fail while suspended")
	}
	if resp, _ := auth.RegisterUser(context.Background(), &authv1.RegisterUserRequest{
		Username: "u2", Email: "u2@example.com", Password: "password123",
		ClientId: appID, ClientSecret: appSecret,
	}); resp.Success {
		t.Fatalf("expected registration to fail while suspended")
	}
	if resp, _ := auth.ValidateToken(context.Background(), &authv1.ValidateTokenRequest{
		AccessToken: loginResp.AccessToken, ClientId: appID, ClientSecret: appSecret,
	}); resp.Valid {
		t.Fatalf("expected token validation to fail while suspended")
	}

	// Restore re-enables auth
	rest, _ := plat.RestoreClient(context.Background(), &authv1.RestoreClientRequest{
		AccessToken: root.AccessToken, ClientId: appID,
	})
	if !rest.Success {
		t.Fatalf("expected RestoreClient to succeed: %s", rest.Message)
	}
	if resp, _ := auth.GetToken(context.Background(), &authv1.GetTokenRequest{
		Email: "u1@example.com", Password: "password123",
		ClientId: appID, ClientSecret: appSecret,
	}); !resp.Success {
		t.Fatalf("expected login to succeed after restore: %s", resp.Message)
	}

	// The platform client itself can never be suspended
	if resp, _ := plat.SuspendClient(context.Background(), &authv1.SuspendClientRequest{
		AccessToken: root.AccessToken, ClientId: PlatformClientID,
	}); resp.Success {
		t.Fatalf("expected suspending the platform client to be refused")
	}
}

func TestOrgScopedSSOLogin(t *testing.T) {
	_, plat, auth := setupPlatform(t)
	registerDeveloper(t, plat, "alice@example.com", "password123", "")
	alice := developerLogin(t, plat, "alice@example.com", "password123")

	// Two org-scoped apps in the same org share a user pool
	app1 := createApp(t, plat, alice.AccessToken, "app-one", models.ScopeOrg)
	app2 := createApp(t, plat, alice.AccessToken, "app-two", models.ScopeOrg)
	// One app-scoped app stays isolated
	app3 := createApp(t, plat, alice.AccessToken, "app-three", models.ScopeApp)

	reg, _ := auth.RegisterUser(context.Background(), &authv1.RegisterUserRequest{
		Username: "carol", Email: "carol@example.com", Password: "password123",
		ClientId: app1.App.ClientId, ClientSecret: app1.ClientSecret,
	})
	if !reg.Success {
		t.Fatalf("expected registration via app-one to succeed: %s", reg.Message)
	}

	// Same credentials log in via the sibling org-scoped app (SSO)
	viaApp2, _ := auth.GetToken(context.Background(), &authv1.GetTokenRequest{
		Email: "carol@example.com", Password: "password123",
		ClientId: app2.App.ClientId, ClientSecret: app2.ClientSecret,
	})
	if !viaApp2.Success {
		t.Fatalf("expected org-scoped SSO login via app-two to succeed: %s", viaApp2.Message)
	}
	if viaApp2.User.UserId != reg.UserId {
		t.Fatalf("expected the same user across org-scoped apps")
	}

	// But not via the app-scoped app (isolated user base)
	viaApp3, _ := auth.GetToken(context.Background(), &authv1.GetTokenRequest{
		Email: "carol@example.com", Password: "password123",
		ClientId: app3.App.ClientId, ClientSecret: app3.ClientSecret,
	})
	if viaApp3.Success {
		t.Fatalf("expected login via app-scoped app to fail (isolated users)")
	}

	// Duplicate email within the org pool is rejected even via the other app
	dup, _ := auth.RegisterUser(context.Background(), &authv1.RegisterUserRequest{
		Username: "carol2", Email: "carol@example.com", Password: "password123",
		ClientId: app2.App.ClientId, ClientSecret: app2.ClientSecret,
	})
	if dup.Success {
		t.Fatalf("expected duplicate email within the org pool to fail")
	}
}

func TestMinimalCheck(t *testing.T) {
	_, plat, _ := setupPlatform(t)
	registerDeveloper(t, plat, "alice@example.com", "password123", "")
	registerDeveloper(t, plat, "bob@example.com", "password123", "")
	alice := developerLogin(t, plat, "alice@example.com", "password123")
	bob := developerLogin(t, plat, "bob@example.com", "password123")

	app := createApp(t, plat, alice.AccessToken, "app", "")
	appID := app.App.ClientId

	check := func(token, clientID, subject, relation, objectType, objectID string) *authv1.CheckResponse {
		t.Helper()
		resp, err := plat.Check(context.Background(), &authv1.CheckRequest{
			AccessToken: token,
			ClientId:    clientID,
			Subject:     subject,
			Relation:    relation,
			ObjectType:  objectType,
			ObjectId:    objectID,
		})
		if err != nil {
			t.Fatalf("Check returned error: %v", err)
		}
		return resp
	}

	// Default deny: no tuples at all
	if resp := check(alice.AccessToken, appID, "user:u1", "editor", "doc", "1"); resp.Allowed {
		t.Fatalf("expected default deny, got allowed (%s)", resp.Reason)
	}

	// Direct allow tuple
	write, _ := plat.WriteTuples(context.Background(), &authv1.WriteTuplesRequest{
		AccessToken: alice.AccessToken, ClientId: appID,
		Tuples: []*authv1.Tuple{{ObjectType: "doc", ObjectId: "1", Relation: "editor", SubjectType: "user", SubjectId: "u1"}},
	})
	if !write.Success {
		t.Fatalf("WriteTuples failed: %s", write.Message)
	}
	if resp := check(alice.AccessToken, appID, "user:u1", "editor", "doc", "1"); !resp.Allowed {
		t.Fatalf("expected direct allow, got deny (%s)", resp.Reason)
	}

	// Exact match only — different relation/object still denied
	if resp := check(alice.AccessToken, appID, "user:u1", "viewer", "doc", "1"); resp.Allowed {
		t.Fatalf("expected no implication expansion in Phase 2")
	}
	if resp := check(alice.AccessToken, appID, "user:u1", "editor", "doc", "2"); resp.Allowed {
		t.Fatalf("expected other objects to stay denied")
	}

	// Deny tuple wins over the allow
	write, _ = plat.WriteTuples(context.Background(), &authv1.WriteTuplesRequest{
		AccessToken: alice.AccessToken, ClientId: appID,
		Tuples: []*authv1.Tuple{{ObjectType: "doc", ObjectId: "1", Relation: "editor", SubjectType: "user", SubjectId: "u1", Effect: models.EffectDeny}},
	})
	if !write.Success {
		t.Fatalf("WriteTuples (deny) failed: %s", write.Message)
	}
	if resp := check(alice.AccessToken, appID, "user:u1", "editor", "doc", "1"); resp.Allowed {
		t.Fatalf("expected deny tuple to win")
	}

	// Removing the deny restores the allow
	del, _ := plat.DeleteTuples(context.Background(), &authv1.DeleteTuplesRequest{
		AccessToken: alice.AccessToken, ClientId: appID,
		Tuples: []*authv1.Tuple{{ObjectType: "doc", ObjectId: "1", Relation: "editor", SubjectType: "user", SubjectId: "u1", Effect: models.EffectDeny}},
	})
	if !del.Success {
		t.Fatalf("DeleteTuples failed: %s", del.Message)
	}
	if resp := check(alice.AccessToken, appID, "user:u1", "editor", "doc", "1"); !resp.Allowed {
		t.Fatalf("expected allow after deleting the deny tuple")
	}

	// Conditioned allow fails closed in Phase 2
	write, _ = plat.WriteTuples(context.Background(), &authv1.WriteTuplesRequest{
		AccessToken: alice.AccessToken, ClientId: appID,
		Tuples: []*authv1.Tuple{{ObjectType: "doc", ObjectId: "9", Relation: "viewer", SubjectType: "user", SubjectId: "u1", ConditionExpr: "ip == '10.0.0.1'"}},
	})
	if !write.Success {
		t.Fatalf("WriteTuples (conditioned) failed: %s", write.Message)
	}
	if resp := check(alice.AccessToken, appID, "user:u1", "viewer", "doc", "9"); resp.Allowed {
		t.Fatalf("expected conditioned allow tuple to fail closed in Phase 2")
	}

	// Malformed subject denied
	if resp := check(alice.AccessToken, appID, "u1", "editor", "doc", "1"); resp.Allowed {
		t.Fatalf("expected malformed subject to be denied")
	}

	// Cross-tenant: bob cannot check in alice's app scope
	if resp := check(bob.AccessToken, appID, "user:u1", "editor", "doc", "1"); resp.Allowed {
		t.Fatalf("expected bob's Check in alice's scope to be denied")
	}

	// ListTuples filter works
	list, _ := plat.ListTuples(context.Background(), &authv1.ListTuplesRequest{
		AccessToken: alice.AccessToken, ClientId: appID,
		Filter: &authv1.TupleFilter{ObjectType: "doc", ObjectId: "1"},
	})
	if !list.Success || len(list.Tuples) != 1 || list.Tuples[0].Relation != "editor" {
		t.Fatalf("expected filtered ListTuples to return the doc:1 tuple, got %+v", list)
	}
}

func TestAuthzModelVersioning(t *testing.T) {
	_, plat, _ := setupPlatform(t)
	registerDeveloper(t, plat, "alice@example.com", "password123", "")
	alice := developerLogin(t, plat, "alice@example.com", "password123")
	app := createApp(t, plat, alice.AccessToken, "app", "")

	// No model yet: success with empty model_json (console maps to null)
	got, _ := plat.GetAuthzModel(context.Background(), &authv1.GetAuthzModelRequest{
		AccessToken: alice.AccessToken, ClientId: app.App.ClientId,
	})
	if !got.Success || got.ModelJson != "" || got.Version != 0 {
		t.Fatalf("expected empty model before first write, got %+v", got)
	}

	// Malformed JSON rejected
	bad, _ := plat.WriteAuthzModel(context.Background(), &authv1.WriteAuthzModelRequest{
		AccessToken: alice.AccessToken, ClientId: app.App.ClientId, ModelJson: "{not json",
	})
	if bad.Success {
		t.Fatalf("expected malformed model JSON to be rejected")
	}

	// Versions increment; Get returns the latest
	v1, _ := plat.WriteAuthzModel(context.Background(), &authv1.WriteAuthzModelRequest{
		AccessToken: alice.AccessToken, ClientId: app.App.ClientId, ModelJson: `{"v":1}`,
	})
	if !v1.Success || v1.Version != 1 {
		t.Fatalf("expected first model version 1, got %+v", v1)
	}
	v2, _ := plat.WriteAuthzModel(context.Background(), &authv1.WriteAuthzModelRequest{
		AccessToken: alice.AccessToken, ClientId: app.App.ClientId, ModelJson: `{"v":2}`,
	})
	if !v2.Success || v2.Version != 2 {
		t.Fatalf("expected second model version 2, got %+v", v2)
	}
	got, _ = plat.GetAuthzModel(context.Background(), &authv1.GetAuthzModelRequest{
		AccessToken: alice.AccessToken, ClientId: app.App.ClientId,
	})
	if !got.Success || got.Version != 2 || got.ModelJson != `{"v":2}` {
		t.Fatalf("expected latest model v2, got %+v", got)
	}
}

func TestSuperadminRPCGating(t *testing.T) {
	t.Setenv("SUPERADMIN_EMAILS", "root@example.com, other@example.com")
	db, plat, auth := setupPlatform(t)

	// Register a plain developer first; BootstrapSuperadmins must not grant
	// anything to them.
	registerDeveloper(t, plat, "alice@example.com", "password123", "")
	if err := BootstrapSuperadmins(context.Background(), db); err != nil {
		t.Fatalf("BootstrapSuperadmins failed: %v", err)
	}
	alice := developerLogin(t, plat, "alice@example.com", "password123")
	if alice.IsSuperadmin {
		t.Fatalf("expected alice not to be superadmin")
	}

	// Every superadmin RPC rejects the plain developer
	if resp, _ := plat.ListAllOrgs(context.Background(), &authv1.ListAllOrgsRequest{AccessToken: alice.AccessToken}); resp.Success {
		t.Fatalf("expected ListAllOrgs to be superadmin-gated")
	}
	if resp, _ := plat.ListAllApps(context.Background(), &authv1.ListAllAppsRequest{AccessToken: alice.AccessToken}); resp.Success {
		t.Fatalf("expected ListAllApps to be superadmin-gated")
	}
	if resp, _ := plat.ListDevelopers(context.Background(), &authv1.ListDevelopersRequest{AccessToken: alice.AccessToken}); resp.Success {
		t.Fatalf("expected ListDevelopers to be superadmin-gated")
	}
	if resp, _ := plat.GetPlatformMetrics(context.Background(), &authv1.GetPlatformMetricsRequest{AccessToken: alice.AccessToken}); resp.Success {
		t.Fatalf("expected GetPlatformMetrics to be superadmin-gated")
	}

	// A developer registering with a listed email is granted on signup
	registerDeveloper(t, plat, "root@example.com", "password123", "")
	root := developerLogin(t, plat, "root@example.com", "password123")
	if !root.IsSuperadmin {
		t.Fatalf("expected root to be superadmin via SUPERADMIN_EMAILS")
	}

	// Superadmin sees everything, including other orgs' apps
	appResp := createApp(t, plat, alice.AccessToken, "alices-app", "")
	orgs, _ := plat.ListAllOrgs(context.Background(), &authv1.ListAllOrgsRequest{AccessToken: root.AccessToken})
	if !orgs.Success || len(orgs.Orgs) != 3 { // platform + alice + root
		t.Fatalf("expected 3 orgs, got %+v", orgs)
	}
	apps, _ := plat.ListAllApps(context.Background(), &authv1.ListAllAppsRequest{AccessToken: root.AccessToken})
	if !apps.Success || len(apps.Apps) != 2 { // platform client + alice's app
		t.Fatalf("expected 2 apps, got %+v", apps)
	}
	devs, _ := plat.ListDevelopers(context.Background(), &authv1.ListDevelopersRequest{AccessToken: root.AccessToken})
	if !devs.Success || len(devs.Developers) != 2 {
		t.Fatalf("expected 2 developers, got %+v", devs)
	}

	// Superadmin bypasses ownership on other orgs' apps
	rotated, _ := plat.RotateAppSecret(context.Background(), &authv1.RotateAppSecretRequest{
		AccessToken: root.AccessToken, ClientId: appResp.App.ClientId,
	})
	if !rotated.Success {
		t.Fatalf("expected superadmin to rotate any app's secret: %s", rotated.Message)
	}

	// Metrics: registering an end user + session bumps the counts
	if resp, _ := auth.RegisterUser(context.Background(), &authv1.RegisterUserRequest{
		Username: "u1", Email: "u1@example.com", Password: "password123",
		ClientId: appResp.App.ClientId, ClientSecret: rotated.ClientSecret,
	}); !resp.Success {
		t.Fatalf("expected user registration to succeed: %s", resp.Message)
	}
	metrics, _ := plat.GetPlatformMetrics(context.Background(), &authv1.GetPlatformMetricsRequest{AccessToken: root.AccessToken})
	if !metrics.Success {
		t.Fatalf("expected metrics to succeed: %s", metrics.Message)
	}
	// developers: alice+root; orgs: platform+2; apps: platform+alice's;
	// users: 2 developers + 1 end user; sessions: alice+root (developer
	// logins); tuples: root's superadmin grant
	if metrics.Developers != 2 || metrics.Orgs != 3 || metrics.Apps != 2 ||
		metrics.Users != 3 || metrics.ActiveSessions != 2 || metrics.Tuples != 1 {
		t.Fatalf("unexpected metrics: %+v", metrics)
	}
}
