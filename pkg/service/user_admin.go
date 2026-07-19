package service

import (
	"authservice/pkg/models"
	authv1 "authservice/proto/auth/v1"
	"context"
	"fmt"
	"log"

	"google.golang.org/protobuf/types/known/timestamppb"
)

// App user management (spec "App user management", added 2026-07-19).
// Developers manage their app's end users from the console; superadmins get
// the same on any app. Every RPC here authenticates a developer token and
// enforces app ownership via authorizeAppAccess (own org OR superadmin).

// authorizeAppUserAdmin is the shared gate for the user-admin RPCs:
// developer token + app ownership (or superadmin).
func (s *PlatformServiceServerImpl) authorizeAppUserAdmin(ctx context.Context, accessToken, clientID string) (*models.Client, error) {
	dev, err := s.authenticateDeveloper(ctx, accessToken)
	if err != nil {
		return nil, fmt.Errorf("invalid token: %w", err)
	}
	client, err := s.authorizeAppAccess(ctx, dev, clientID)
	if err != nil {
		return nil, fmt.Errorf("ownership check failed (developer: %s): %w", dev.DeveloperID, err)
	}
	return client, nil
}

// requireAppScopedUser loads the target user and verifies it actually
// belongs to the app's identity scope (its own user base, or — for
// org-scoped apps — the org's shared pool). Users of other apps/orgs are
// reported as not found (no existence oracle).
func (s *PlatformServiceServerImpl) requireAppScopedUser(ctx context.Context, client *models.Client, userID string) (*models.User, error) {
	if userID == "" {
		return nil, fmt.Errorf("user_id is required")
	}
	user, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("user not found")
	}
	scopeType, scopeID := userScope(client)
	if user.ScopeType != scopeType || user.ScopeID != scopeID {
		return nil, fmt.Errorf("user is outside the app's identity scope")
	}
	return user, nil
}

func appUserToProto(u *models.User, scope string, sessionCount int64) *authv1.AppUser {
	return &authv1.AppUser{
		UserId:       u.UserID,
		Username:     u.UserName,
		Email:        u.Email,
		CreatedAt:    timestamppb.New(u.CreatedAt),
		Active:       u.Active,
		SessionCount: int32(sessionCount),
		Scope:        scope,
	}
}

func (s *PlatformServiceServerImpl) ListAppUsers(ctx context.Context, req *authv1.ListAppUsersRequest) (*authv1.ListAppUsersResponse, error) {
	log.Printf("ListAppUsers request received for client: %s", req.ClientId)

	fail := func(msg string) (*authv1.ListAppUsersResponse, error) {
		return &authv1.ListAppUsersResponse{Success: false, Message: msg}, nil
	}

	client, err := s.authorizeAppUserAdmin(ctx, req.AccessToken, req.ClientId)
	if err != nil {
		log.Printf("ListAppUsers rejected for client %s: %v", req.ClientId, err)
		return fail("App not found")
	}

	// Users of the app: its own app-scoped base, PLUS (org-scoped apps only)
	// the org's shared pool — labeled so the console can flag that
	// deactivating an org user affects the whole org.
	users, err := s.repo.ListUsersByScope(ctx, models.ScopeApp, client.ClientID, req.Query)
	if err != nil {
		log.Printf("ListAppUsers: app-scoped listing failed for client %s: %v", client.ClientID, err)
		return fail("Internal server error")
	}
	if client.IdentityScope == models.ScopeOrg {
		orgUsers, oerr := s.repo.ListUsersByScope(ctx, models.ScopeOrg, client.OrgID, req.Query)
		if oerr != nil {
			log.Printf("ListAppUsers: org-scoped listing failed for client %s: %v", client.ClientID, oerr)
			return fail("Internal server error")
		}
		users = append(users, orgUsers...)
	}

	counts, err := s.repo.CountSessionsByUserForClient(ctx, client.ClientID)
	if err != nil {
		log.Printf("ListAppUsers: session count failed for client %s: %v", client.ClientID, err)
		return fail("Internal server error")
	}

	out := make([]*authv1.AppUser, 0, len(users))
	for i := range users {
		// The user row's own ScopeType is the label: "app" (this app's base)
		// or "org" (the org's shared pool).
		out = append(out, appUserToProto(&users[i], users[i].ScopeType, counts[users[i].UserID]))
	}
	return &authv1.ListAppUsersResponse{Success: true, Message: "Users retrieved successfully", Users: out}, nil
}

func (s *PlatformServiceServerImpl) ListUserSessionsAdmin(ctx context.Context, req *authv1.ListUserSessionsAdminRequest) (*authv1.ListUserSessionsAdminResponse, error) {
	log.Printf("ListUserSessionsAdmin request received for client: %s", req.ClientId)

	fail := func(msg string) (*authv1.ListUserSessionsAdminResponse, error) {
		return &authv1.ListUserSessionsAdminResponse{Success: false, Message: msg}, nil
	}

	client, err := s.authorizeAppUserAdmin(ctx, req.AccessToken, req.ClientId)
	if err != nil {
		log.Printf("ListUserSessionsAdmin rejected for client %s: %v", req.ClientId, err)
		return fail("App not found")
	}
	user, err := s.requireAppScopedUser(ctx, client, req.UserId)
	if err != nil {
		log.Printf("ListUserSessionsAdmin: target check failed (client: %s): %v", client.ClientID, err)
		return fail("User not found")
	}

	// currentSessionID="" — the admin is never the session owner.
	sessions, err := listUserSessionInfos(ctx, s.repo, user.UserID, client.ClientID, "")
	if err != nil {
		log.Printf("ListUserSessionsAdmin: session listing failed for user %s: %v", user.UserID, err)
		return fail("Internal server error")
	}
	return &authv1.ListUserSessionsAdminResponse{Success: true, Message: "Sessions retrieved successfully", Sessions: sessions}, nil
}

func (s *PlatformServiceServerImpl) RevokeUserSessionsAdmin(ctx context.Context, req *authv1.RevokeUserSessionsAdminRequest) (*authv1.RevokeUserSessionsAdminResponse, error) {
	log.Printf("RevokeUserSessionsAdmin request received for client: %s", req.ClientId)

	fail := func(msg string) (*authv1.RevokeUserSessionsAdminResponse, error) {
		return &authv1.RevokeUserSessionsAdminResponse{Success: false, Message: msg}, nil
	}

	client, err := s.authorizeAppUserAdmin(ctx, req.AccessToken, req.ClientId)
	if err != nil {
		log.Printf("RevokeUserSessionsAdmin rejected for client %s: %v", req.ClientId, err)
		return fail("App not found")
	}
	user, err := s.requireAppScopedUser(ctx, client, req.UserId)
	if err != nil {
		log.Printf("RevokeUserSessionsAdmin: target check failed (client: %s): %v", client.ClientID, err)
		return fail("User not found")
	}

	revoked, err := s.repo.DeleteUserClientSessions(ctx, user.UserID, client.ClientID)
	if err != nil {
		log.Printf("RevokeUserSessionsAdmin: deletion failed for user %s: %v", user.UserID, err)
		return fail("Internal server error")
	}

	log.Printf("Admin revoked all sessions for user %s (client: %s, count: %d)", user.UserID, client.ClientID, revoked)
	return &authv1.RevokeUserSessionsAdminResponse{
		Success:      true,
		Message:      "All of the user's sessions for this app have been revoked",
		RevokedCount: int32(revoked),
	}, nil
}

func (s *PlatformServiceServerImpl) SetUserActive(ctx context.Context, req *authv1.SetUserActiveRequest) (*authv1.SetUserActiveResponse, error) {
	log.Printf("SetUserActive request received for client: %s (active: %v)", req.ClientId, req.Active)

	fail := func(msg string) (*authv1.SetUserActiveResponse, error) {
		return &authv1.SetUserActiveResponse{Success: false, Message: msg}, nil
	}

	client, err := s.authorizeAppUserAdmin(ctx, req.AccessToken, req.ClientId)
	if err != nil {
		log.Printf("SetUserActive rejected for client %s: %v", req.ClientId, err)
		return fail("App not found")
	}
	user, err := s.requireAppScopedUser(ctx, client, req.UserId)
	if err != nil {
		log.Printf("SetUserActive: target check failed (client: %s): %v", client.ClientID, err)
		return fail("User not found")
	}

	if err := s.repo.SetUserActive(ctx, user.UserID, req.Active); err != nil {
		log.Printf("SetUserActive: update failed for user %s: %v", user.UserID, err)
		return fail("Internal server error")
	}
	user.Active = req.Active

	msg := "User reactivated; they can log in again"
	if !req.Active {
		// Deactivation disables the IDENTITY, not one app's access, so every
		// session dies — for org-scoped users that spans all of the org's
		// apps (they share one user row; a session under any sibling app
		// belongs to the same disabled identity).
		if err := s.repo.DeleteAllUserSessions(ctx, user.UserID); err != nil {
			log.Printf("SetUserActive: session revocation failed for user %s: %v", user.UserID, err)
			return fail("Internal server error")
		}
		msg = "User deactivated; all of their sessions have been revoked"
		if user.ScopeType == models.ScopeOrg {
			msg = "User deactivated; this is an org-scoped identity, so they are disabled and logged out across ALL of the org's apps"
		}
	}

	counts, err := s.repo.CountSessionsByUserForClient(ctx, client.ClientID)
	if err != nil {
		counts = nil // non-fatal; count degrades to 0
	}

	log.Printf("SetUserActive: user %s active=%v (client: %s)", user.UserID, req.Active, client.ClientID)
	return &authv1.SetUserActiveResponse{
		Success: true,
		Message: msg,
		User:    appUserToProto(user, user.ScopeType, counts[user.UserID]),
	}, nil
}
