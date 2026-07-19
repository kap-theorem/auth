package service

import (
	"authservice/pkg/authz"
	"authservice/pkg/models"
	"authservice/pkg/repository"
	"authservice/pkg/utils"
	authv1 "authservice/proto/auth/v1"
	"context"
	"fmt"
	"log"
	"net/url"
	"os"
	"slices"
	"strings"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"
	"gorm.io/gorm"
)

// PlatformClientID is the well-known client_id of the built-in platform
// client. Developers are users of this client (dogfooding); the console's
// superadmin check targets the object app:platform in this client's scope.
const PlatformClientID = "platform"

// Superadmin tuple coordinates (spec §8): app:platform admin user:<dev>.
const (
	platformObjectType   = "app"
	platformObjectID     = "platform"
	platformSuperRel     = "admin"
	tupleSubjectTypeUser = "user"
)

// PlatformServiceServerImpl implements the Phase 2 tenant-management and
// minimal-authz RPCs. Developer auth reuses the AuthService user/session/JWT
// machinery via the built-in platform client.
type PlatformServiceServerImpl struct {
	authv1.UnimplementedPlatformServiceServer
	repo     *repository.AuthRepository
	resolver *authz.Resolver
}

func NewPlatformServiceServer(db *gorm.DB) *PlatformServiceServerImpl {
	repo := repository.NewAuthRepository(db)
	return &PlatformServiceServerImpl{
		repo:     repo,
		resolver: authz.NewResolver(repo),
	}
}

// ---------------------------------------------------------------------------
// Bootstrap
// ---------------------------------------------------------------------------

// BootstrapPlatform ensures the built-in platform org + client exist. The
// client secret comes from PLATFORM_CLIENT_SECRET; when unset in dev a
// secret is generated and logged ONCE. If the env secret changes, the stored
// hash is re-synced so the env var stays the source of truth.
func BootstrapPlatform(ctx context.Context, db *gorm.DB) error {
	repo := repository.NewAuthRepository(db)
	secret := os.Getenv("PLATFORM_CLIENT_SECRET")

	client, err := repo.GetClientByID(ctx, PlatformClientID)
	if err == nil {
		// Already bootstrapped; re-sync the secret if the env var rotated it.
		if secret != "" && !utils.CheckPasswordHash(secret, client.ClientSecretHash) {
			hash, herr := utils.HashPassword(secret)
			if herr != nil {
				return fmt.Errorf("hashing platform client secret: %w", herr)
			}
			if uerr := repo.UpdateClientSecretHash(ctx, PlatformClientID, hash); uerr != nil {
				return fmt.Errorf("updating platform client secret: %w", uerr)
			}
			log.Println("Platform client secret re-synced from PLATFORM_CLIENT_SECRET")
		}
		return nil
	}

	if secret == "" {
		if !strings.EqualFold(os.Getenv("ENV"), "dev") {
			return fmt.Errorf("PLATFORM_CLIENT_SECRET must be set (generated secrets are only allowed when ENV=dev)")
		}
		generated, gerr := utils.GenerateClientSecret()
		if gerr != nil {
			return fmt.Errorf("generating platform client secret: %w", gerr)
		}
		secret = generated
		// Dev convenience: logged once at creation, never again.
		log.Printf("Generated platform client secret (dev only, shown once): %s", secret)
	}

	hash, err := utils.HashPassword(secret)
	if err != nil {
		return fmt.Errorf("hashing platform client secret: %w", err)
	}

	org := &models.Organization{
		OrgID: utils.GenerateUUID(),
		Name:  "platform",
	}
	if err := repo.CreateOrganization(ctx, org); err != nil {
		return fmt.Errorf("creating platform org: %w", err)
	}

	if err := repo.CreateClient(ctx, &models.Client{
		ClientID:         PlatformClientID,
		ClientName:       "platform",
		ClientSecretHash: hash,
		OrgID:            org.OrgID,
		IdentityScope:    models.ScopeOrg,
	}); err != nil {
		return fmt.Errorf("creating platform client: %w", err)
	}

	log.Println("Platform org and client bootstrapped")
	return nil
}

// superadminEmails parses the SUPERADMIN_EMAILS env var (comma-separated).
func superadminEmails() map[string]bool {
	emails := map[string]bool{}
	for _, e := range strings.Split(os.Getenv("SUPERADMIN_EMAILS"), ",") {
		if e = strings.TrimSpace(e); e != "" {
			emails[strings.ToLower(e)] = true
		}
	}
	return emails
}

// grantSuperadmin ensures the (app:platform, admin, user:<dev>) allow tuple.
func grantSuperadmin(ctx context.Context, repo *repository.AuthRepository, developerID string) error {
	return repo.UpsertTuples(ctx, []models.RelationTuple{{
		ClientID:    PlatformClientID,
		ObjectType:  platformObjectType,
		ObjectID:    platformObjectID,
		Relation:    platformSuperRel,
		SubjectType: tupleSubjectTypeUser,
		SubjectID:   developerID,
		Effect:      models.EffectAllow,
	}})
}

// BootstrapSuperadmins seeds the superadmin tuple for every SUPERADMIN_EMAILS
// entry that matches an existing developer. Unknown emails are skipped (the
// grant is re-checked when a developer registers with a listed email).
func BootstrapSuperadmins(ctx context.Context, db *gorm.DB) error {
	repo := repository.NewAuthRepository(db)
	for email := range superadminEmails() {
		dev, err := repo.GetDeveloperByEmail(ctx, email)
		if err != nil {
			continue // not registered yet
		}
		if err := grantSuperadmin(ctx, repo, dev.DeveloperID); err != nil {
			return fmt.Errorf("granting superadmin to developer %s: %w", dev.DeveloperID, err)
		}
		log.Printf("Superadmin tuple ensured for developer: %s", dev.DeveloperID)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

// authenticateDeveloper validates a platform-client access token
// (revocation-aware) and returns the developer record.
func (s *PlatformServiceServerImpl) authenticateDeveloper(ctx context.Context, accessToken string) (*models.Developer, error) {
	claims, err := utils.ValidateJWTToken(accessToken)
	if err != nil {
		return nil, fmt.Errorf("token validation failed: %w", err)
	}
	if claims.ClientID != PlatformClientID {
		return nil, fmt.Errorf("token is not a platform token")
	}
	session, err := s.repo.GetSessionByID(ctx, claims.SessionID)
	if err != nil {
		return nil, fmt.Errorf("session lookup failed: %w", err)
	}
	if session.UserID != claims.Subject || session.ClientID != claims.ClientID {
		return nil, fmt.Errorf("session does not match token claims")
	}
	dev, err := s.repo.GetDeveloperByID(ctx, claims.Subject)
	if err != nil {
		return nil, fmt.Errorf("developer lookup failed: %w", err)
	}
	return dev, nil
}

// loadModel returns the parsed latest authz model for a client, or nil when
// no model has been written. A stored model that no longer parses (written
// before schema validation existed) degrades to nil — exact-relation
// resolution only, matching the console mock's tolerance.
func (s *PlatformServiceServerImpl) loadModel(ctx context.Context, clientID string) *authz.Model {
	stored, err := s.repo.GetLatestAuthzModel(ctx, clientID)
	if err != nil {
		return nil // no model written yet
	}
	model, perr := authz.ParseModel(stored.ModelJSON)
	if perr != nil {
		log.Printf("loadModel: stored model v%d for client %s does not parse (ignoring implications): %v", stored.Version, clientID, perr)
		return nil
	}
	return model
}

// checkResolved runs the full Phase 3 resolver (spec §6) against the
// client's latest authz model: deny pass first (exact relation), allow pass
// with implication expansion and role/userset hops, conditions fail closed,
// depth-limited, default deny.
func (s *PlatformServiceServerImpl) checkResolved(ctx context.Context, clientID, subjectType, subjectID, relation, objectType, objectID string, condCtx map[string]string) (bool, string, error) {
	model := s.loadModel(ctx, clientID)
	return s.resolver.Check(ctx, clientID, model, subjectType, subjectID, relation, objectType, objectID, condCtx)
}

// isSuperadmin runs the spec §8 check: Check(platform, caller, admin,
// app:platform).
func (s *PlatformServiceServerImpl) isSuperadmin(ctx context.Context, developerID string) bool {
	allowed, _, err := s.checkResolved(ctx, PlatformClientID, tupleSubjectTypeUser, developerID, platformSuperRel, platformObjectType, platformObjectID, nil)
	if err != nil {
		log.Printf("Superadmin check failed for developer %s: %v", developerID, err)
		return false
	}
	return allowed
}

// authorizeAppAccess loads an app and verifies the developer owns it (same
// org) or is a superadmin.
func (s *PlatformServiceServerImpl) authorizeAppAccess(ctx context.Context, dev *models.Developer, clientID string) (*models.Client, error) {
	if clientID == "" {
		return nil, fmt.Errorf("client_id is required")
	}
	client, err := s.repo.GetClientByID(ctx, clientID)
	if err != nil {
		return nil, fmt.Errorf("app not found")
	}
	if client.OrgID != dev.OrgID && !s.isSuperadmin(ctx, dev.DeveloperID) {
		return nil, fmt.Errorf("app does not belong to the developer's org")
	}
	return client, nil
}

// requireSuperadmin authenticates the token and enforces the superadmin
// check server-side (UI hiding is never the security boundary).
func (s *PlatformServiceServerImpl) requireSuperadmin(ctx context.Context, accessToken string) (*models.Developer, error) {
	dev, err := s.authenticateDeveloper(ctx, accessToken)
	if err != nil {
		return nil, err
	}
	if !s.isSuperadmin(ctx, dev.DeveloperID) {
		return nil, fmt.Errorf("superadmin access required")
	}
	return dev, nil
}

func appToProto(c *models.Client) *authv1.App {
	return &authv1.App{
		ClientId:        c.ClientID,
		Name:            c.ClientName,
		OrgId:           c.OrgID,
		IdentityScope:   c.IdentityScope,
		Suspended:       c.Suspended,
		CreatedAt:       timestamppb.New(c.CreatedAt),
		RedirectUris:    c.GetRedirectURIs(),
		DemoEnabled:     c.DemoEnabled,
		LoginIdentifier: c.LoginIdentifier,
		PublicSignup:    c.PublicSignup,
	}
}

// validLoginIdentifier reports whether m is a recognized login-identifier mode.
func validLoginIdentifier(m string) bool {
	switch m {
	case "username_or_email", "email_only", "username_only":
		return true
	}
	return false
}

// normalizeRedirectURIs trims, drops empties, dedupes, and validates that
// every whitelisted hosted-login redirect is an absolute URL.
func normalizeRedirectURIs(uris []string) ([]string, error) {
	out := make([]string, 0, len(uris))
	for _, raw := range uris {
		u := strings.TrimSpace(raw)
		if u == "" {
			continue
		}
		parsed, err := url.Parse(u)
		if err != nil || parsed.Scheme == "" || parsed.Host == "" {
			return nil, fmt.Errorf("redirect URI %q must be an absolute URL", u)
		}
		if !slices.Contains(out, u) {
			out = append(out, u)
		}
	}
	return out, nil
}

func validIdentityScope(scope string) bool {
	return scope == models.ScopeApp || scope == models.ScopeOrg
}

// validateTuple checks the required tuple fields and normalizes the effect.
func validateTuple(t *authv1.Tuple) (effect string, err error) {
	if t.ObjectType == "" || t.ObjectId == "" || t.Relation == "" || t.SubjectType == "" || t.SubjectId == "" {
		return "", fmt.Errorf("object_type, object_id, relation, subject_type, and subject_id are required")
	}
	switch t.Effect {
	case "", models.EffectAllow:
		return models.EffectAllow, nil
	case models.EffectDeny:
		return models.EffectDeny, nil
	default:
		return "", fmt.Errorf("effect must be 'allow' or 'deny'")
	}
}

// ---------------------------------------------------------------------------
// Developer accounts
// ---------------------------------------------------------------------------

func (s *PlatformServiceServerImpl) RegisterDeveloper(ctx context.Context, req *authv1.RegisterDeveloperRequest) (*authv1.RegisterDeveloperResponse, error) {
	log.Printf("RegisterDeveloper request received")

	email := strings.ToLower(strings.TrimSpace(req.Email))
	fail := func(msg string) (*authv1.RegisterDeveloperResponse, error) {
		return &authv1.RegisterDeveloperResponse{Success: false, Message: msg}, nil
	}

	if email == "" || !isValidEmail(email) {
		return fail("A valid email is required")
	}
	if len(req.Password) < 8 {
		return fail("Password must be at least 8 characters long")
	}

	platform, err := s.repo.GetClientByID(ctx, PlatformClientID)
	if err != nil {
		log.Printf("RegisterDeveloper: platform client missing: %v", err)
		return fail("Internal server error")
	}

	// Developers live in the platform org's shared user pool.
	exists, err := s.repo.IsEmailExists(ctx, email, models.ScopeOrg, platform.OrgID)
	if err != nil {
		log.Printf("RegisterDeveloper: email existence check failed: %v", err)
		return fail("Internal server error")
	}
	if exists {
		return fail("Email already registered")
	}

	hashedPassword, err := utils.HashPassword(req.Password)
	if err != nil {
		log.Printf("RegisterDeveloper: password hash failed: %v", err)
		return fail("Internal server error")
	}

	// Personal org, auto-created (GitHub-style).
	orgName := strings.TrimSpace(req.OrgName)
	if orgName == "" {
		orgName = strings.SplitN(email, "@", 2)[0]
	}
	org := &models.Organization{OrgID: utils.GenerateUUID(), Name: orgName}
	if err := s.repo.CreateOrganization(ctx, org); err != nil {
		log.Printf("RegisterDeveloper: org creation failed: %v", err)
		return fail("Internal server error")
	}

	userID := utils.GenerateUUID()
	if err := s.repo.CreateUser(ctx, &models.User{
		UserID:    userID,
		UserName:  orgName,
		Email:     email,
		Password:  hashedPassword,
		ScopeType: models.ScopeOrg,
		ScopeID:   platform.OrgID,
	}); err != nil {
		log.Printf("RegisterDeveloper: user creation failed: %v", err)
		return fail("Internal server error")
	}

	if err := s.repo.CreateDeveloper(ctx, &models.Developer{
		DeveloperID: userID,
		Email:       email,
		OrgID:       org.OrgID,
	}); err != nil {
		log.Printf("RegisterDeveloper: developer creation failed: %v", err)
		return fail("Internal server error")
	}

	// Re-run the superadmin bootstrap check for freshly registered emails.
	if superadminEmails()[email] {
		if err := grantSuperadmin(ctx, s.repo, userID); err != nil {
			log.Printf("RegisterDeveloper: superadmin grant failed for developer %s: %v", userID, err)
		} else {
			log.Printf("Superadmin tuple granted to developer: %s", userID)
		}
	}

	log.Printf("Developer registered successfully: %s (org: %s)", userID, org.OrgID)
	return &authv1.RegisterDeveloperResponse{
		Success:     true,
		Message:     "Developer registered successfully",
		DeveloperId: userID,
		OrgId:       org.OrgID,
	}, nil
}

func (s *PlatformServiceServerImpl) DeveloperLogin(ctx context.Context, req *authv1.DeveloperLoginRequest) (*authv1.DeveloperLoginResponse, error) {
	log.Printf("DeveloperLogin request received")

	email := strings.ToLower(strings.TrimSpace(req.Email))
	fail := func(msg string) (*authv1.DeveloperLoginResponse, error) {
		return &authv1.DeveloperLoginResponse{Success: false, Message: msg}, nil
	}

	if email == "" || req.Password == "" {
		return fail("Email and password are required")
	}

	platform, err := s.repo.GetClientByID(ctx, PlatformClientID)
	if err != nil {
		log.Printf("DeveloperLogin: platform client missing: %v", err)
		return fail("Internal server error")
	}

	user, err := s.repo.GetUserByEmail(ctx, email, models.ScopeOrg, platform.OrgID)
	if err != nil {
		log.Printf("DeveloperLogin failed: user lookup error")
		return fail("Invalid credentials")
	}
	if !utils.CheckPasswordHash(req.Password, user.Password) {
		log.Printf("DeveloperLogin failed for user %s", user.UserID)
		return fail("Invalid credentials")
	}

	dev, err := s.repo.GetDeveloperByID(ctx, user.UserID)
	if err != nil {
		log.Printf("DeveloperLogin failed: developer record missing for user %s", user.UserID)
		return fail("Invalid credentials")
	}

	// Session + tokens: same machinery as end-user logins (dogfooding).
	sessionID := utils.GenerateUUID()
	refreshToken, refreshHash, err := utils.GenerateRefreshToken(sessionID)
	if err != nil {
		log.Printf("DeveloperLogin: refresh token generation failed: %v", err)
		return fail("Internal server error")
	}
	if err := s.repo.CreateSession(ctx, &models.Session{
		SessionID:        sessionID,
		UserID:           user.UserID,
		ClientID:         PlatformClientID,
		RefreshTokenHash: refreshHash,
		UserAgent:        req.UserAgent,
		ExpiresAt:        time.Now().Add(refreshTokenLifetime),
	}); err != nil {
		log.Printf("DeveloperLogin: session creation failed: %v", err)
		return fail("Internal server error")
	}

	accessToken, _, err := utils.GenerateJWTToken(user.UserID, PlatformClientID, sessionID)
	if err != nil {
		log.Printf("DeveloperLogin: JWT generation failed: %v", err)
		return fail("Internal server error")
	}

	orgName := ""
	if org, err := s.repo.GetOrganizationByID(ctx, dev.OrgID); err == nil {
		orgName = org.Name
	}

	log.Printf("Developer logged in successfully: %s (session: %s)", user.UserID, sessionID)
	return &authv1.DeveloperLoginResponse{
		Success:      true,
		Message:      "Login successful",
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		Developer: &authv1.Developer{
			DeveloperId: dev.DeveloperID,
			Email:       dev.Email,
			OrgId:       dev.OrgID,
			OrgName:     orgName,
		},
		IsSuperadmin: s.isSuperadmin(ctx, dev.DeveloperID),
	}, nil
}

// DeveloperRefreshToken refreshes a developer session. It wraps the
// AuthService refresh machinery on the built-in platform client using the
// service's own authority — the browser never holds a client secret — with
// the same rotation and reuse-revocation semantics: presenting a rotated
// (stale) refresh token is a theft signal that revokes the whole session.
func (s *PlatformServiceServerImpl) DeveloperRefreshToken(ctx context.Context, req *authv1.DeveloperRefreshTokenRequest) (*authv1.DeveloperRefreshTokenResponse, error) {
	log.Printf("DeveloperRefreshToken request received")

	fail := func(msg string) (*authv1.DeveloperRefreshTokenResponse, error) {
		return &authv1.DeveloperRefreshTokenResponse{Success: false, Message: msg}, nil
	}

	if req.RefreshToken == "" {
		return fail("Refresh token is required")
	}

	// Parse opaque token: "<session_id>.<secret>"
	sessionID, secret, ok := utils.ParseRefreshToken(req.RefreshToken)
	if !ok {
		return fail("Invalid refresh token")
	}

	session, err := s.repo.GetSessionByID(ctx, sessionID)
	if err != nil {
		log.Printf("DeveloperRefreshToken failed: session not found")
		return fail("Invalid refresh token")
	}

	// Only platform-client (developer) sessions may be refreshed here.
	if session.ClientID != PlatformClientID {
		log.Printf("DeveloperRefreshToken failed: session %s is not a platform session", session.SessionID)
		return fail("Invalid refresh token")
	}

	// A mismatch on an existing session means a rotated (stale) token was
	// replayed — treat as theft and revoke the whole session.
	if !utils.VerifyRefreshSecret(secret, session.RefreshTokenHash) {
		log.Printf("DeveloperRefreshToken: refresh token reuse detected; revoking session %s (user: %s)", session.SessionID, session.UserID)
		if err := s.repo.DeleteSessionByID(ctx, session.SessionID); err != nil {
			log.Printf("DeveloperRefreshToken: error revoking session after reuse detection: %v", err)
		}
		return fail("Invalid refresh token")
	}

	// Rotate: new secret, new hash, sliding expiry.
	newRefreshToken, newHash, err := utils.GenerateRefreshToken(session.SessionID)
	if err != nil {
		log.Printf("DeveloperRefreshToken: refresh token generation failed: %v", err)
		return fail("Internal server error")
	}
	session.RefreshTokenHash = newHash
	session.ExpiresAt = time.Now().Add(refreshTokenLifetime)
	if err := s.repo.UpdateSession(ctx, session); err != nil {
		log.Printf("DeveloperRefreshToken: session update failed: %v", err)
		return fail("Internal server error")
	}

	accessToken, _, err := utils.GenerateJWTToken(session.UserID, PlatformClientID, session.SessionID)
	if err != nil {
		log.Printf("DeveloperRefreshToken: JWT generation failed: %v", err)
		return fail("Internal server error")
	}

	log.Printf("Developer token refreshed successfully (session: %s)", session.SessionID)
	return &authv1.DeveloperRefreshTokenResponse{
		Success:      true,
		Message:      "Token refreshed successfully",
		AccessToken:  accessToken,
		RefreshToken: newRefreshToken,
	}, nil
}

// ---------------------------------------------------------------------------
// App management
// ---------------------------------------------------------------------------

func (s *PlatformServiceServerImpl) CreateApp(ctx context.Context, req *authv1.CreateAppRequest) (*authv1.CreateAppResponse, error) {
	log.Printf("CreateApp request received")

	fail := func(msg string) (*authv1.CreateAppResponse, error) {
		return &authv1.CreateAppResponse{Success: false, Message: msg}, nil
	}

	dev, err := s.authenticateDeveloper(ctx, req.AccessToken)
	if err != nil {
		log.Printf("CreateApp: developer authentication failed: %v", err)
		return fail("Invalid token")
	}

	if strings.TrimSpace(req.Name) == "" {
		return fail("App name is required")
	}
	identityScope := req.IdentityScope
	if identityScope == "" {
		identityScope = models.ScopeApp
	}
	if !validIdentityScope(identityScope) {
		return fail("identity_scope must be 'app' or 'org'")
	}

	clientID := utils.GenerateUUID()
	clientSecret, err := utils.GenerateClientSecret()
	if err != nil {
		log.Printf("CreateApp: secret generation failed: %v", err)
		return fail("Internal server error")
	}
	secretHash, err := utils.HashPassword(clientSecret)
	if err != nil {
		log.Printf("CreateApp: secret hash failed: %v", err)
		return fail("Internal server error")
	}

	client := &models.Client{
		ClientID:         clientID,
		ClientName:       strings.TrimSpace(req.Name),
		ClientSecretHash: secretHash,
		OrgID:            dev.OrgID,
		IdentityScope:    identityScope,
	}
	if err := s.repo.CreateClient(ctx, client); err != nil {
		log.Printf("CreateApp: client creation failed: %v", err)
		return fail("Failed to create app")
	}

	log.Printf("App created: %s (org: %s, developer: %s)", clientID, dev.OrgID, dev.DeveloperID)
	return &authv1.CreateAppResponse{
		Success:      true,
		Message:      "App created successfully",
		App:          appToProto(client),
		ClientSecret: clientSecret, // shown once; only the bcrypt hash is stored
	}, nil
}

func (s *PlatformServiceServerImpl) ListApps(ctx context.Context, req *authv1.ListAppsRequest) (*authv1.ListAppsResponse, error) {
	log.Printf("ListApps request received")

	dev, err := s.authenticateDeveloper(ctx, req.AccessToken)
	if err != nil {
		log.Printf("ListApps: developer authentication failed: %v", err)
		return &authv1.ListAppsResponse{Success: false, Message: "Invalid token"}, nil
	}

	clients, err := s.repo.ListClientsByOrg(ctx, dev.OrgID)
	if err != nil {
		log.Printf("ListApps: listing failed for org %s: %v", dev.OrgID, err)
		return &authv1.ListAppsResponse{Success: false, Message: "Internal server error"}, nil
	}

	apps := make([]*authv1.App, 0, len(clients))
	for i := range clients {
		apps = append(apps, appToProto(&clients[i]))
	}
	return &authv1.ListAppsResponse{Success: true, Message: "Apps retrieved successfully", Apps: apps}, nil
}

func (s *PlatformServiceServerImpl) UpdateApp(ctx context.Context, req *authv1.UpdateAppRequest) (*authv1.UpdateAppResponse, error) {
	log.Printf("UpdateApp request received for client: %s", req.ClientId)

	fail := func(msg string) (*authv1.UpdateAppResponse, error) {
		return &authv1.UpdateAppResponse{Success: false, Message: msg}, nil
	}

	dev, err := s.authenticateDeveloper(ctx, req.AccessToken)
	if err != nil {
		log.Printf("UpdateApp: developer authentication failed: %v", err)
		return fail("Invalid token")
	}

	client, err := s.authorizeAppAccess(ctx, dev, req.ClientId)
	if err != nil {
		log.Printf("UpdateApp: ownership check failed for client %s (developer: %s): %v", req.ClientId, dev.DeveloperID, err)
		return fail("App not found")
	}

	if req.Name != nil {
		if strings.TrimSpace(*req.Name) == "" {
			return fail("App name cannot be empty")
		}
		client.ClientName = strings.TrimSpace(*req.Name)
	}
	if req.IdentityScope != nil {
		if !validIdentityScope(*req.IdentityScope) {
			return fail("identity_scope must be 'app' or 'org'")
		}
		client.IdentityScope = *req.IdentityScope
	}
	if req.SetRedirectUris {
		uris, uerr := normalizeRedirectURIs(req.RedirectUris)
		if uerr != nil {
			return fail(uerr.Error())
		}
		if serr := client.SetRedirectURIs(uris); serr != nil {
			log.Printf("UpdateApp: redirect URI encoding failed for client %s: %v", req.ClientId, serr)
			return fail("Internal server error")
		}
	}
	if req.DemoEnabled != nil {
		client.DemoEnabled = *req.DemoEnabled
	}
	if req.DemoEmail != nil {
		client.DemoEmail = strings.TrimSpace(*req.DemoEmail)
	}
	if req.DemoPassword != nil {
		client.DemoPassword = *req.DemoPassword
	}
	if req.LoginIdentifier != nil {
		if !validLoginIdentifier(*req.LoginIdentifier) {
			return fail("login_identifier must be 'username_or_email', 'email_only', or 'username_only'")
		}
		client.LoginIdentifier = *req.LoginIdentifier
	}
	if req.PublicSignup != nil {
		client.PublicSignup = *req.PublicSignup
	}
	// Guard: enabling demo login requires demo credentials to be present.
	if client.DemoEnabled && (client.DemoEmail == "" || client.DemoPassword == "") {
		return fail("demo login requires a demo email and password")
	}

	if err := s.repo.UpdateClient(ctx, client); err != nil {
		log.Printf("UpdateApp: update failed for client %s: %v", req.ClientId, err)
		return fail("Internal server error")
	}

	log.Printf("App updated: %s (developer: %s)", client.ClientID, dev.DeveloperID)
	return &authv1.UpdateAppResponse{Success: true, Message: "App updated successfully", App: appToProto(client)}, nil
}

func (s *PlatformServiceServerImpl) RotateAppSecret(ctx context.Context, req *authv1.RotateAppSecretRequest) (*authv1.RotateAppSecretResponse, error) {
	log.Printf("RotateAppSecret request received for client: %s", req.ClientId)

	fail := func(msg string) (*authv1.RotateAppSecretResponse, error) {
		return &authv1.RotateAppSecretResponse{Success: false, Message: msg}, nil
	}

	dev, err := s.authenticateDeveloper(ctx, req.AccessToken)
	if err != nil {
		log.Printf("RotateAppSecret: developer authentication failed: %v", err)
		return fail("Invalid token")
	}

	client, err := s.authorizeAppAccess(ctx, dev, req.ClientId)
	if err != nil {
		log.Printf("RotateAppSecret: ownership check failed for client %s (developer: %s): %v", req.ClientId, dev.DeveloperID, err)
		return fail("App not found")
	}

	newSecret, err := utils.GenerateClientSecret()
	if err != nil {
		log.Printf("RotateAppSecret: secret generation failed: %v", err)
		return fail("Internal server error")
	}
	newHash, err := utils.HashPassword(newSecret)
	if err != nil {
		log.Printf("RotateAppSecret: secret hash failed: %v", err)
		return fail("Internal server error")
	}
	if err := s.repo.UpdateClientSecretHash(ctx, client.ClientID, newHash); err != nil {
		log.Printf("RotateAppSecret: update failed for client %s: %v", client.ClientID, err)
		return fail("Internal server error")
	}

	log.Printf("App secret rotated: %s (developer: %s)", client.ClientID, dev.DeveloperID)
	return &authv1.RotateAppSecretResponse{
		Success:      true,
		Message:      "App secret rotated successfully",
		ClientSecret: newSecret, // shown once; only the bcrypt hash is stored
	}, nil
}

func (s *PlatformServiceServerImpl) DeleteApp(ctx context.Context, req *authv1.DeleteAppRequest) (*authv1.DeleteAppResponse, error) {
	log.Printf("DeleteApp request received for client: %s", req.ClientId)

	fail := func(msg string) (*authv1.DeleteAppResponse, error) {
		return &authv1.DeleteAppResponse{Success: false, Message: msg}, nil
	}

	dev, err := s.authenticateDeveloper(ctx, req.AccessToken)
	if err != nil {
		log.Printf("DeleteApp: developer authentication failed: %v", err)
		return fail("Invalid token")
	}

	if req.ClientId == PlatformClientID {
		return fail("The platform client cannot be deleted")
	}

	client, err := s.authorizeAppAccess(ctx, dev, req.ClientId)
	if err != nil {
		log.Printf("DeleteApp: ownership check failed for client %s (developer: %s): %v", req.ClientId, dev.DeveloperID, err)
		return fail("App not found")
	}

	if err := s.repo.DeleteClient(ctx, client.ClientID); err != nil {
		log.Printf("DeleteApp: delete failed for client %s: %v", client.ClientID, err)
		return fail("Internal server error")
	}

	log.Printf("App deleted: %s (developer: %s)", client.ClientID, dev.DeveloperID)
	return &authv1.DeleteAppResponse{Success: true, Message: "App deleted successfully"}, nil
}

// ---------------------------------------------------------------------------
// Hosted login (spec "Hosted login", added 2026-07-18)
// ---------------------------------------------------------------------------

// GetAppPublicInfo is UNAUTHENTICATED: it exposes only the app's display
// name and whether hosted login is enabled. Unknown client ids yield
// {success: false} and nothing else (no existence oracle beyond that).
func (s *PlatformServiceServerImpl) GetAppPublicInfo(ctx context.Context, req *authv1.GetAppPublicInfoRequest) (*authv1.GetAppPublicInfoResponse, error) {
	log.Printf("GetAppPublicInfo request received for client: %s", req.ClientId)

	if req.ClientId == "" {
		return &authv1.GetAppPublicInfoResponse{Success: false}, nil
	}
	client, err := s.repo.GetClientByID(ctx, req.ClientId)
	if err != nil {
		return &authv1.GetAppPublicInfoResponse{Success: false}, nil
	}
	hostedEnabled := len(client.GetRedirectURIs()) > 0 && !client.Suspended
	loginID := client.LoginIdentifier
	if loginID == "" {
		loginID = "username_or_email"
	}
	return &authv1.GetAppPublicInfoResponse{
		Success:            true,
		Name:               client.ClientName,
		HostedLoginEnabled: hostedEnabled,
		// Demo/signup buttons only make sense when hosted pages are enabled.
		DemoEnabled:     hostedEnabled && client.DemoEnabled && client.DemoEmail != "",
		LoginIdentifier: loginID,
		PublicSignup:    hostedEnabled && client.PublicSignup,
	}, nil
}

// HostedLogin is the client-secret-free login path behind the
// platform-hosted login page. The redirect_uri whitelist is the trust
// anchor: the call succeeds only when redirect_uri EXACTLY matches one of
// the app's whitelisted redirect_uris (string equality, no prefix
// matching) and the app is not suspended. Reuses the GetToken machinery
// (identity scoping included); rate-limited by the interceptor with the
// same per-email login limiter as GetToken.
func (s *PlatformServiceServerImpl) HostedLogin(ctx context.Context, req *authv1.HostedLoginRequest) (*authv1.HostedLoginResponse, error) {
	log.Printf("HostedLogin request received for client: %s", req.ClientId)

	fail := func(msg string) (*authv1.HostedLoginResponse, error) {
		return &authv1.HostedLoginResponse{Success: false, Message: msg}, nil
	}

	if req.ClientId == "" || req.Email == "" || req.Password == "" || req.RedirectUri == "" {
		return fail("Email, password, client ID, and redirect URI are required")
	}

	client, err := s.hostedEnabledClient(ctx, req.ClientId)
	if err != nil {
		// Same generic message as every other rejection below: unknown app,
		// suspended app, and unlisted redirect are indistinguishable.
		log.Printf("HostedLogin rejected for client %s: %v", req.ClientId, err)
		return fail("Hosted login is not available for this app")
	}
	if !slices.Contains(client.GetRedirectURIs(), req.RedirectUri) {
		log.Printf("HostedLogin rejected: redirect_uri not whitelisted (client: %s)", client.ClientID)
		return fail("Hosted login is not available for this app")
	}

	resp, err := issueLoginTokens(ctx, s.repo, client, req.Email, req.Password, req.UserAgent)
	if err != nil {
		return nil, err
	}
	return &authv1.HostedLoginResponse{
		Success:      resp.Success,
		Message:      resp.Message,
		AccessToken:  resp.AccessToken,
		RefreshToken: resp.RefreshToken,
		ExpiresAt:    resp.ExpiresAt,
		User:         resp.User,
		SessionId:    resp.SessionId,
	}, nil
}

// hostedClientForRedirect is the shared trust gate for the secret-free hosted
// login/demo/register RPCs: hosted pages enabled AND redirect_uri exactly
// whitelisted. Returns the same generic error for every rejection so unknown
// app / suspended / unlisted-redirect are indistinguishable.
func (s *PlatformServiceServerImpl) hostedClientForRedirect(ctx context.Context, clientID, redirectURI string) (*models.Client, error) {
	client, err := s.hostedEnabledClient(ctx, clientID)
	if err != nil {
		return nil, err
	}
	if !slices.Contains(client.GetRedirectURIs(), redirectURI) {
		return nil, fmt.Errorf("redirect_uri not whitelisted")
	}
	return client, nil
}

// HostedDemoLogin logs in the app's configured demo account with one click.
// The demo password is stored server-side and never sent to the browser. The
// demo user is resolved by email regardless of the app's login-identifier mode.
func (s *PlatformServiceServerImpl) HostedDemoLogin(ctx context.Context, req *authv1.HostedDemoLoginRequest) (*authv1.HostedDemoLoginResponse, error) {
	log.Printf("HostedDemoLogin request received for client: %s", req.ClientId)

	fail := func(msg string) (*authv1.HostedDemoLoginResponse, error) {
		return &authv1.HostedDemoLoginResponse{Success: false, Message: msg}, nil
	}

	if req.ClientId == "" || req.RedirectUri == "" {
		return fail("Client ID and redirect URI are required")
	}
	client, err := s.hostedClientForRedirect(ctx, req.ClientId, req.RedirectUri)
	if err != nil {
		log.Printf("HostedDemoLogin rejected for client %s: %v", req.ClientId, err)
		return fail("Demo login is not available for this app")
	}
	if !client.DemoEnabled || client.DemoEmail == "" || client.DemoPassword == "" {
		return fail("Demo login is not available for this app")
	}

	scopeType, scopeID := userScope(client)
	user, err := s.repo.GetUserByEmail(ctx, client.DemoEmail, scopeType, scopeID)
	if err != nil {
		log.Printf("HostedDemoLogin: demo user lookup failed for client %s: %v", req.ClientId, err)
		return fail("Demo login is not available for this app")
	}
	resp, err := finishLogin(ctx, s.repo, client, user, client.DemoPassword, req.UserAgent)
	if err != nil {
		return nil, err
	}
	return &authv1.HostedDemoLoginResponse{
		Success:      resp.Success,
		Message:      resp.Message,
		AccessToken:  resp.AccessToken,
		RefreshToken: resp.RefreshToken,
		ExpiresAt:    resp.ExpiresAt,
		User:         resp.User,
		SessionId:    resp.SessionId,
	}, nil
}

// HostedRegister is the secret-free public signup path. Enabled per app via
// public_signup; trust anchored on the redirect whitelist (like HostedLogin).
// On success the new user is logged in.
func (s *PlatformServiceServerImpl) HostedRegister(ctx context.Context, req *authv1.HostedRegisterRequest) (*authv1.HostedRegisterResponse, error) {
	log.Printf("HostedRegister request received for client: %s", req.ClientId)

	fail := func(msg string) (*authv1.HostedRegisterResponse, error) {
		return &authv1.HostedRegisterResponse{Success: false, Message: msg}, nil
	}

	if req.ClientId == "" || req.RedirectUri == "" {
		return fail("Client ID and redirect URI are required")
	}
	if strings.TrimSpace(req.Username) == "" || strings.TrimSpace(req.Email) == "" {
		return fail("Username and email are required")
	}
	if len(req.Password) < 8 {
		return fail("Password must be at least 8 characters")
	}
	client, err := s.hostedClientForRedirect(ctx, req.ClientId, req.RedirectUri)
	if err != nil {
		log.Printf("HostedRegister rejected for client %s: %v", req.ClientId, err)
		return fail("Signup is not available for this app")
	}
	if !client.PublicSignup {
		return fail("Signup is not available for this app")
	}

	userID, msg, ok := createEndUser(ctx, s.repo, client, strings.TrimSpace(req.Username), strings.TrimSpace(req.Email), req.Password)
	if !ok {
		return fail(msg)
	}
	log.Printf("HostedRegister: user %s created for client %s; auto-logging in", userID, req.ClientId)

	// Auto-login the freshly created user.
	user, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		return fail("Account created; please sign in")
	}
	resp, err := finishLogin(ctx, s.repo, client, user, req.Password, req.UserAgent)
	if err != nil {
		return nil, err
	}
	return &authv1.HostedRegisterResponse{
		Success:      resp.Success,
		Message:      resp.Message,
		AccessToken:  resp.AccessToken,
		RefreshToken: resp.RefreshToken,
		ExpiresAt:    resp.ExpiresAt,
		User:         resp.User,
		SessionId:    resp.SessionId,
	}, nil
}

// ---------------------------------------------------------------------------
// Hosted account page (spec "Hosted account page", added 2026-07-18)
// ---------------------------------------------------------------------------

// hostedEnabledClient is the shared hosted-pages gate (HostedLogin and every
// Hosted* account RPC): the app must exist, not be suspended, and have a
// non-empty redirect whitelist — the whitelist is the hosted opt-in signal,
// so hosted pages exist only for apps that opted in.
func (s *PlatformServiceServerImpl) hostedEnabledClient(ctx context.Context, clientID string) (*models.Client, error) {
	if clientID == "" {
		return nil, fmt.Errorf("client_id is required")
	}
	client, err := s.repo.GetClientByID(ctx, clientID)
	if err != nil {
		return nil, fmt.Errorf("app not found")
	}
	if client.Suspended {
		return nil, fmt.Errorf("app is suspended")
	}
	if len(client.GetRedirectURIs()) == 0 {
		return nil, fmt.Errorf("hosted pages are not enabled for this app")
	}
	return client, nil
}

// authenticateHostedUser gates a hosted account RPC: hosted pages enabled
// for the app AND the end-user access token belongs to THIS client
// (validateUserSessionToken enforces token client_id == request client_id
// plus revocation-aware session validation).
func (s *PlatformServiceServerImpl) authenticateHostedUser(ctx context.Context, clientID, accessToken string) (*utils.Claims, *models.Session, error) {
	if _, err := s.hostedEnabledClient(ctx, clientID); err != nil {
		return nil, nil, err
	}
	if accessToken == "" {
		return nil, nil, fmt.Errorf("access token is required")
	}
	return validateUserSessionToken(ctx, s.repo, accessToken, clientID)
}

// HostedGetProfile returns the token owner's profile and active sessions
// for this client. Authenticated purely by the end user's access token.
func (s *PlatformServiceServerImpl) HostedGetProfile(ctx context.Context, req *authv1.HostedGetProfileRequest) (*authv1.HostedGetProfileResponse, error) {
	log.Printf("HostedGetProfile request received for client: %s", req.ClientId)

	fail := func(msg string) (*authv1.HostedGetProfileResponse, error) {
		return &authv1.HostedGetProfileResponse{Success: false, Message: msg}, nil
	}

	claims, current, err := s.authenticateHostedUser(ctx, req.ClientId, req.AccessToken)
	if err != nil {
		log.Printf("HostedGetProfile rejected for client %s: %v", req.ClientId, err)
		return fail("Not signed in")
	}

	user, err := s.repo.GetUserByID(ctx, claims.Subject)
	if err != nil {
		log.Printf("HostedGetProfile: user lookup failed: %v", err)
		return fail("Not signed in")
	}

	sessions, err := listUserSessionInfos(ctx, s.repo, claims.Subject, req.ClientId, current.SessionID)
	if err != nil {
		log.Printf("HostedGetProfile: session listing failed for user %s: %v", claims.Subject, err)
		return fail("Internal server error")
	}

	return &authv1.HostedGetProfileResponse{
		Success: true,
		Message: "Profile retrieved successfully",
		User: &authv1.UserProfile{
			UserId:    user.UserID,
			Username:  user.UserName,
			Email:     user.Email,
			ClientId:  req.ClientId,
			CreatedAt: timestamppb.New(user.CreatedAt),
		},
		Sessions: sessions,
	}, nil
}

// HostedChangePassword changes the token owner's password with the same
// semantics as AuthService.ChangeUserPassword: current password verified,
// >=8 chars, bcrypt, all OTHER sessions invalidated (the caller's kept).
func (s *PlatformServiceServerImpl) HostedChangePassword(ctx context.Context, req *authv1.HostedChangePasswordRequest) (*authv1.HostedChangePasswordResponse, error) {
	log.Printf("HostedChangePassword request received for client: %s", req.ClientId)

	claims, current, err := s.authenticateHostedUser(ctx, req.ClientId, req.AccessToken)
	if err != nil {
		log.Printf("HostedChangePassword rejected for client %s: %v", req.ClientId, err)
		return &authv1.HostedChangePasswordResponse{Success: false, Message: "Not signed in"}, nil
	}

	// Respect an admin-set password lock.
	if user, uerr := s.repo.GetUserByID(ctx, claims.Subject); uerr == nil && user.LockPassword {
		return &authv1.HostedChangePasswordResponse{Success: false, Message: "Password changes are disabled for this account"}, nil
	}

	ok, msg := changeUserPassword(ctx, s.repo, claims.Subject, current.SessionID, req.CurrentPassword, req.NewPassword)
	return &authv1.HostedChangePasswordResponse{Success: ok, Message: msg}, nil
}

// HostedUpdateProfile lets the token owner change their own username and/or
// email, honoring admin-set locks and per-scope uniqueness. Unset fields are
// left unchanged.
func (s *PlatformServiceServerImpl) HostedUpdateProfile(ctx context.Context, req *authv1.HostedUpdateProfileRequest) (*authv1.HostedUpdateProfileResponse, error) {
	log.Printf("HostedUpdateProfile request received for client: %s", req.ClientId)

	fail := func(msg string) (*authv1.HostedUpdateProfileResponse, error) {
		return &authv1.HostedUpdateProfileResponse{Success: false, Message: msg}, nil
	}

	claims, _, err := s.authenticateHostedUser(ctx, req.ClientId, req.AccessToken)
	if err != nil {
		log.Printf("HostedUpdateProfile rejected for client %s: %v", req.ClientId, err)
		return fail("Not signed in")
	}
	user, err := s.repo.GetUserByID(ctx, claims.Subject)
	if err != nil {
		return fail("Not signed in")
	}

	if req.Username != nil {
		name := strings.TrimSpace(*req.Username)
		if name != user.UserName {
			if user.LockUsername {
				return fail("Username changes are disabled for this account")
			}
			if name == "" {
				return fail("Username cannot be empty")
			}
			taken, cerr := s.repo.IsUsernameExists(ctx, name, user.ScopeType, user.ScopeID, user.UserID)
			if cerr != nil {
				return fail("Internal server error")
			}
			if taken {
				return fail("Username already taken")
			}
			user.UserName = name
		}
	}
	if req.Email != nil {
		email := strings.TrimSpace(*req.Email)
		if email != user.Email {
			if user.LockEmail {
				return fail("Email changes are disabled for this account")
			}
			if !isValidEmail(email) {
				return fail("A valid email is required")
			}
			taken, cerr := s.repo.IsEmailExistsExcluding(ctx, email, user.ScopeType, user.ScopeID, user.UserID)
			if cerr != nil {
				return fail("Internal server error")
			}
			if taken {
				return fail("Email already registered")
			}
			user.Email = email
		}
	}

	if err := s.repo.UpdateUser(ctx, user); err != nil {
		log.Printf("HostedUpdateProfile: save failed for user %s: %v", user.UserID, err)
		return fail("Internal server error")
	}
	return &authv1.HostedUpdateProfileResponse{
		Success: true,
		Message: "Profile updated successfully",
		User: &authv1.UserProfile{
			UserId:    user.UserID,
			Username:  user.UserName,
			Email:     user.Email,
			ClientId:  req.ClientId,
			CreatedAt: timestamppb.New(user.CreatedAt),
		},
	}, nil
}

// HostedRevokeSession revokes one of the token owner's OWN sessions under
// this client; anyone else's session id yields "Session not found".
func (s *PlatformServiceServerImpl) HostedRevokeSession(ctx context.Context, req *authv1.HostedRevokeSessionRequest) (*authv1.HostedRevokeSessionResponse, error) {
	log.Printf("HostedRevokeSession request received for client: %s", req.ClientId)

	claims, _, err := s.authenticateHostedUser(ctx, req.ClientId, req.AccessToken)
	if err != nil {
		log.Printf("HostedRevokeSession rejected for client %s: %v", req.ClientId, err)
		return &authv1.HostedRevokeSessionResponse{Success: false, Message: "Not signed in"}, nil
	}

	ok, msg := revokeOwnUserSession(ctx, s.repo, claims.Subject, req.ClientId, req.SessionId)
	return &authv1.HostedRevokeSessionResponse{Success: ok, Message: msg}, nil
}

// HostedLogoutAll revokes ALL of the token owner's sessions for this client,
// including the current one (the presented token dies with it).
func (s *PlatformServiceServerImpl) HostedLogoutAll(ctx context.Context, req *authv1.HostedLogoutAllRequest) (*authv1.HostedLogoutAllResponse, error) {
	log.Printf("HostedLogoutAll request received for client: %s", req.ClientId)

	claims, _, err := s.authenticateHostedUser(ctx, req.ClientId, req.AccessToken)
	if err != nil {
		log.Printf("HostedLogoutAll rejected for client %s: %v", req.ClientId, err)
		return &authv1.HostedLogoutAllResponse{Success: false, Message: "Not signed in"}, nil
	}

	revoked, err := s.repo.DeleteUserClientSessions(ctx, claims.Subject, req.ClientId)
	if err != nil {
		log.Printf("HostedLogoutAll: session deletion failed for user %s: %v", claims.Subject, err)
		return &authv1.HostedLogoutAllResponse{Success: false, Message: "Internal server error"}, nil
	}

	log.Printf("HostedLogoutAll: all sessions revoked for user %s (client: %s, count: %d)", claims.Subject, req.ClientId, revoked)
	return &authv1.HostedLogoutAllResponse{Success: true, Message: "All sessions logged out"}, nil
}

// ---------------------------------------------------------------------------
// Authorization (Phase 3 resolver)
// ---------------------------------------------------------------------------

func (s *PlatformServiceServerImpl) Check(ctx context.Context, req *authv1.CheckRequest) (*authv1.CheckResponse, error) {
	log.Printf("Check request received")

	deny := func(reason string) (*authv1.CheckResponse, error) {
		return &authv1.CheckResponse{Allowed: false, Reason: reason}, nil
	}

	var clientID string
	if req.AccessToken == "" && req.ClientSecret != "" {
		// Machine-to-machine alternative (spec "Hosted login"): client
		// credentials authorize checks scoped to that client ONLY — the
		// credential IS the scope, so a different client_id cannot be named.
		client, err := authenticateClientCreds(ctx, s.repo, req.ClientId, req.ClientSecret)
		if err != nil {
			log.Printf("Check: client authentication failed for client %s: %v", req.ClientId, err)
			return deny("invalid client credentials")
		}
		clientID = client.ClientID
	} else {
		dev, err := s.authenticateDeveloper(ctx, req.AccessToken)
		if err != nil {
			return deny("invalid token")
		}

		// Resolve the tuple scope: empty defaults to the platform client (the
		// console's superadmin gate); any developer may query the platform
		// scope, but per-app scopes require ownership (or superadmin).
		clientID = req.ClientId
		if clientID == "" {
			clientID = PlatformClientID
		}
		if clientID != PlatformClientID {
			if _, err := s.authorizeAppAccess(ctx, dev, clientID); err != nil {
				log.Printf("Check: ownership check failed for client %s (developer: %s): %v", clientID, dev.DeveloperID, err)
				return deny("app not found")
			}
		}
	}

	subjectType, subjectID, ok := strings.Cut(req.Subject, ":")
	if !ok || subjectType == "" || subjectID == "" {
		return deny("subject must be of the form 'type:id'")
	}
	if req.Relation == "" || req.ObjectType == "" || req.ObjectId == "" {
		return deny("relation, object_type, and object_id are required")
	}

	allowed, reason, err := s.checkResolved(ctx, clientID, subjectType, subjectID, req.Relation, req.ObjectType, req.ObjectId, req.Context)
	if err != nil {
		log.Printf("Check: resolution failed (client: %s): %v", clientID, err)
		return deny("internal error (failing closed)")
	}
	return &authv1.CheckResponse{Allowed: allowed, Reason: reason}, nil
}

func (s *PlatformServiceServerImpl) ListObjects(ctx context.Context, req *authv1.ListObjectsRequest) (*authv1.ListObjectsResponse, error) {
	log.Printf("ListObjects request received for client: %s", req.ClientId)

	fail := func(msg string) (*authv1.ListObjectsResponse, error) {
		return &authv1.ListObjectsResponse{Success: false, Message: msg}, nil
	}

	dev, err := s.authenticateDeveloper(ctx, req.AccessToken)
	if err != nil {
		log.Printf("ListObjects: developer authentication failed: %v", err)
		return fail("Invalid token")
	}

	// Same scope rules as Check: empty defaults to the platform client;
	// per-app scopes require ownership (or superadmin).
	clientID := req.ClientId
	if clientID == "" {
		clientID = PlatformClientID
	}
	if clientID != PlatformClientID {
		if _, err := s.authorizeAppAccess(ctx, dev, clientID); err != nil {
			log.Printf("ListObjects: ownership check failed for client %s (developer: %s): %v", clientID, dev.DeveloperID, err)
			return fail("App not found")
		}
	}

	subjectType, subjectID, ok := strings.Cut(req.Subject, ":")
	if !ok || subjectType == "" || subjectID == "" {
		return fail("subject must be of the form 'type:id'")
	}
	if req.Relation == "" || req.ObjectType == "" {
		return fail("relation and object_type are required")
	}

	model := s.loadModel(ctx, clientID)
	objectIDs, err := s.resolver.ListObjects(ctx, clientID, model, subjectType, subjectID, req.ObjectType, req.Relation)
	if err != nil {
		log.Printf("ListObjects: resolution failed (client: %s): %v", clientID, err)
		return fail("Internal server error")
	}

	return &authv1.ListObjectsResponse{
		Success:   true,
		Message:   "Objects retrieved successfully",
		ObjectIds: objectIDs,
	}, nil
}

func (s *PlatformServiceServerImpl) WriteTuples(ctx context.Context, req *authv1.WriteTuplesRequest) (*authv1.WriteTuplesResponse, error) {
	log.Printf("WriteTuples request received for client: %s", req.ClientId)

	fail := func(msg string) (*authv1.WriteTuplesResponse, error) {
		return &authv1.WriteTuplesResponse{Success: false, Message: msg}, nil
	}

	dev, err := s.authenticateDeveloper(ctx, req.AccessToken)
	if err != nil {
		log.Printf("WriteTuples: developer authentication failed: %v", err)
		return fail("Invalid token")
	}
	client, err := s.authorizeAppAccess(ctx, dev, req.ClientId)
	if err != nil {
		log.Printf("WriteTuples: ownership check failed for client %s (developer: %s): %v", req.ClientId, dev.DeveloperID, err)
		return fail("App not found")
	}
	if len(req.Tuples) == 0 {
		return fail("At least one tuple is required")
	}

	tuples := make([]models.RelationTuple, 0, len(req.Tuples))
	for _, t := range req.Tuples {
		effect, verr := validateTuple(t)
		if verr != nil {
			return fail(verr.Error())
		}
		tuples = append(tuples, models.RelationTuple{
			ClientID:      client.ClientID,
			ObjectType:    t.ObjectType,
			ObjectID:      t.ObjectId,
			Relation:      t.Relation,
			SubjectType:   t.SubjectType,
			SubjectID:     t.SubjectId,
			Effect:        effect,
			ConditionExpr: t.ConditionExpr,
		})
	}

	if err := s.repo.UpsertTuples(ctx, tuples); err != nil {
		log.Printf("WriteTuples: write failed for client %s: %v", client.ClientID, err)
		return fail("Internal server error")
	}

	log.Printf("Tuples written: %d (client: %s, developer: %s)", len(tuples), client.ClientID, dev.DeveloperID)
	return &authv1.WriteTuplesResponse{Success: true, Message: "Tuples written successfully"}, nil
}

func (s *PlatformServiceServerImpl) DeleteTuples(ctx context.Context, req *authv1.DeleteTuplesRequest) (*authv1.DeleteTuplesResponse, error) {
	log.Printf("DeleteTuples request received for client: %s", req.ClientId)

	fail := func(msg string) (*authv1.DeleteTuplesResponse, error) {
		return &authv1.DeleteTuplesResponse{Success: false, Message: msg}, nil
	}

	dev, err := s.authenticateDeveloper(ctx, req.AccessToken)
	if err != nil {
		log.Printf("DeleteTuples: developer authentication failed: %v", err)
		return fail("Invalid token")
	}
	client, err := s.authorizeAppAccess(ctx, dev, req.ClientId)
	if err != nil {
		log.Printf("DeleteTuples: ownership check failed for client %s (developer: %s): %v", req.ClientId, dev.DeveloperID, err)
		return fail("App not found")
	}
	if len(req.Tuples) == 0 {
		return fail("At least one tuple is required")
	}

	for _, t := range req.Tuples {
		effect, verr := validateTuple(t)
		if verr != nil {
			return fail(verr.Error())
		}
		if err := s.repo.DeleteTuple(ctx, &models.RelationTuple{
			ClientID:    client.ClientID,
			ObjectType:  t.ObjectType,
			ObjectID:    t.ObjectId,
			Relation:    t.Relation,
			SubjectType: t.SubjectType,
			SubjectID:   t.SubjectId,
			Effect:      effect,
		}); err != nil {
			log.Printf("DeleteTuples: delete failed for client %s: %v", client.ClientID, err)
			return fail("Internal server error")
		}
	}

	log.Printf("Tuples deleted: %d (client: %s, developer: %s)", len(req.Tuples), client.ClientID, dev.DeveloperID)
	return &authv1.DeleteTuplesResponse{Success: true, Message: "Tuples deleted successfully"}, nil
}

func (s *PlatformServiceServerImpl) ListTuples(ctx context.Context, req *authv1.ListTuplesRequest) (*authv1.ListTuplesResponse, error) {
	log.Printf("ListTuples request received for client: %s", req.ClientId)

	fail := func(msg string) (*authv1.ListTuplesResponse, error) {
		return &authv1.ListTuplesResponse{Success: false, Message: msg}, nil
	}

	var client *models.Client
	if req.AccessToken == "" && req.ClientSecret != "" {
		// Machine-to-machine alternative (spec "Hosted login"): client
		// credentials list that client's OWN tuples only — the credential IS
		// the scope, so a different client_id cannot be named.
		c, err := authenticateClientCreds(ctx, s.repo, req.ClientId, req.ClientSecret)
		if err != nil {
			log.Printf("ListTuples: client authentication failed for client %s: %v", req.ClientId, err)
			return fail("Invalid client credentials")
		}
		client = c
	} else {
		dev, err := s.authenticateDeveloper(ctx, req.AccessToken)
		if err != nil {
			log.Printf("ListTuples: developer authentication failed: %v", err)
			return fail("Invalid token")
		}
		client, err = s.authorizeAppAccess(ctx, dev, req.ClientId)
		if err != nil {
			log.Printf("ListTuples: ownership check failed for client %s (developer: %s): %v", req.ClientId, dev.DeveloperID, err)
			return fail("App not found")
		}
	}

	filter := req.Filter
	if filter == nil {
		filter = &authv1.TupleFilter{}
	}
	tuples, err := s.repo.ListTuples(ctx, client.ClientID, filter.ObjectType, filter.ObjectId, filter.SubjectType, filter.SubjectId)
	if err != nil {
		log.Printf("ListTuples: listing failed for client %s: %v", client.ClientID, err)
		return fail("Internal server error")
	}

	out := make([]*authv1.Tuple, 0, len(tuples))
	for _, t := range tuples {
		out = append(out, &authv1.Tuple{
			ObjectType:    t.ObjectType,
			ObjectId:      t.ObjectID,
			Relation:      t.Relation,
			SubjectType:   t.SubjectType,
			SubjectId:     t.SubjectID,
			Effect:        t.Effect,
			ConditionExpr: t.ConditionExpr,
		})
	}
	return &authv1.ListTuplesResponse{Success: true, Message: "Tuples retrieved successfully", Tuples: out}, nil
}

func (s *PlatformServiceServerImpl) WriteAuthzModel(ctx context.Context, req *authv1.WriteAuthzModelRequest) (*authv1.WriteAuthzModelResponse, error) {
	log.Printf("WriteAuthzModel request received for client: %s", req.ClientId)

	fail := func(msg string) (*authv1.WriteAuthzModelResponse, error) {
		return &authv1.WriteAuthzModelResponse{Success: false, Message: msg}, nil
	}

	dev, err := s.authenticateDeveloper(ctx, req.AccessToken)
	if err != nil {
		log.Printf("WriteAuthzModel: developer authentication failed: %v", err)
		return fail("Invalid token")
	}
	client, err := s.authorizeAppAccess(ctx, dev, req.ClientId)
	if err != nil {
		log.Printf("WriteAuthzModel: ownership check failed for client %s (developer: %s): %v", req.ClientId, dev.DeveloperID, err)
		return fail("App not found")
	}

	// Validate against the authz model schema (see pkg/authz/model.go):
	// unknown relations in implication lists and implication cycles are
	// rejected, not just malformed JSON.
	if _, verr := authz.ParseModel(req.ModelJson); verr != nil {
		return fail(fmt.Sprintf("Invalid authz model: %v", verr))
	}

	version, err := s.repo.CreateAuthzModelVersion(ctx, client.ClientID, req.ModelJson)
	if err != nil {
		log.Printf("WriteAuthzModel: write failed for client %s: %v", client.ClientID, err)
		return fail("Internal server error")
	}

	log.Printf("Authz model v%d written (client: %s, developer: %s)", version, client.ClientID, dev.DeveloperID)
	return &authv1.WriteAuthzModelResponse{Success: true, Message: "Authz model written successfully", Version: version}, nil
}

func (s *PlatformServiceServerImpl) GetAuthzModel(ctx context.Context, req *authv1.GetAuthzModelRequest) (*authv1.GetAuthzModelResponse, error) {
	log.Printf("GetAuthzModel request received for client: %s", req.ClientId)

	fail := func(msg string) (*authv1.GetAuthzModelResponse, error) {
		return &authv1.GetAuthzModelResponse{Success: false, Message: msg}, nil
	}

	dev, err := s.authenticateDeveloper(ctx, req.AccessToken)
	if err != nil {
		log.Printf("GetAuthzModel: developer authentication failed: %v", err)
		return fail("Invalid token")
	}
	client, err := s.authorizeAppAccess(ctx, dev, req.ClientId)
	if err != nil {
		log.Printf("GetAuthzModel: ownership check failed for client %s (developer: %s): %v", req.ClientId, dev.DeveloperID, err)
		return fail("App not found")
	}

	model, err := s.repo.GetLatestAuthzModel(ctx, client.ClientID)
	if err != nil {
		// No model yet is not an error: the console maps an empty
		// model_json to null.
		return &authv1.GetAuthzModelResponse{Success: true, Message: "No authz model written yet"}, nil
	}
	return &authv1.GetAuthzModelResponse{
		Success:   true,
		Message:   "Authz model retrieved successfully",
		ModelJson: model.ModelJSON,
		Version:   model.Version,
	}, nil
}

// ---------------------------------------------------------------------------
// Superadmin-only RPCs
// ---------------------------------------------------------------------------

func (s *PlatformServiceServerImpl) ListAllOrgs(ctx context.Context, req *authv1.ListAllOrgsRequest) (*authv1.ListAllOrgsResponse, error) {
	log.Printf("ListAllOrgs request received")

	if _, err := s.requireSuperadmin(ctx, req.AccessToken); err != nil {
		log.Printf("ListAllOrgs rejected: %v", err)
		return &authv1.ListAllOrgsResponse{Success: false, Message: "Superadmin access required"}, nil
	}

	orgs, err := s.repo.ListOrganizations(ctx)
	if err != nil {
		log.Printf("ListAllOrgs: listing failed: %v", err)
		return &authv1.ListAllOrgsResponse{Success: false, Message: "Internal server error"}, nil
	}

	out := make([]*authv1.Org, 0, len(orgs))
	for _, o := range orgs {
		out = append(out, &authv1.Org{
			OrgId:     o.OrgID,
			Name:      o.Name,
			CreatedAt: timestamppb.New(o.CreatedAt),
		})
	}
	return &authv1.ListAllOrgsResponse{Success: true, Message: "Orgs retrieved successfully", Orgs: out}, nil
}

func (s *PlatformServiceServerImpl) ListAllApps(ctx context.Context, req *authv1.ListAllAppsRequest) (*authv1.ListAllAppsResponse, error) {
	log.Printf("ListAllApps request received")

	if _, err := s.requireSuperadmin(ctx, req.AccessToken); err != nil {
		log.Printf("ListAllApps rejected: %v", err)
		return &authv1.ListAllAppsResponse{Success: false, Message: "Superadmin access required"}, nil
	}

	clients, err := s.repo.ListAllClients(ctx)
	if err != nil {
		log.Printf("ListAllApps: listing failed: %v", err)
		return &authv1.ListAllAppsResponse{Success: false, Message: "Internal server error"}, nil
	}

	apps := make([]*authv1.App, 0, len(clients))
	for i := range clients {
		apps = append(apps, appToProto(&clients[i]))
	}
	return &authv1.ListAllAppsResponse{Success: true, Message: "Apps retrieved successfully", Apps: apps}, nil
}

func (s *PlatformServiceServerImpl) SuspendClient(ctx context.Context, req *authv1.SuspendClientRequest) (*authv1.SuspendClientResponse, error) {
	log.Printf("SuspendClient request received for client: %s", req.ClientId)

	fail := func(msg string) (*authv1.SuspendClientResponse, error) {
		return &authv1.SuspendClientResponse{Success: false, Message: msg}, nil
	}

	if _, err := s.requireSuperadmin(ctx, req.AccessToken); err != nil {
		log.Printf("SuspendClient rejected: %v", err)
		return fail("Superadmin access required")
	}
	if req.ClientId == PlatformClientID {
		return fail("The platform client cannot be suspended")
	}
	if _, err := s.repo.GetClientByID(ctx, req.ClientId); err != nil {
		return fail("App not found")
	}
	if err := s.repo.SetClientSuspended(ctx, req.ClientId, true); err != nil {
		log.Printf("SuspendClient: update failed for client %s: %v", req.ClientId, err)
		return fail("Internal server error")
	}

	log.Printf("Client suspended: %s", req.ClientId)
	return &authv1.SuspendClientResponse{Success: true, Message: "Client suspended"}, nil
}

func (s *PlatformServiceServerImpl) RestoreClient(ctx context.Context, req *authv1.RestoreClientRequest) (*authv1.RestoreClientResponse, error) {
	log.Printf("RestoreClient request received for client: %s", req.ClientId)

	fail := func(msg string) (*authv1.RestoreClientResponse, error) {
		return &authv1.RestoreClientResponse{Success: false, Message: msg}, nil
	}

	if _, err := s.requireSuperadmin(ctx, req.AccessToken); err != nil {
		log.Printf("RestoreClient rejected: %v", err)
		return fail("Superadmin access required")
	}
	if _, err := s.repo.GetClientByID(ctx, req.ClientId); err != nil {
		return fail("App not found")
	}
	if err := s.repo.SetClientSuspended(ctx, req.ClientId, false); err != nil {
		log.Printf("RestoreClient: update failed for client %s: %v", req.ClientId, err)
		return fail("Internal server error")
	}

	log.Printf("Client restored: %s", req.ClientId)
	return &authv1.RestoreClientResponse{Success: true, Message: "Client restored"}, nil
}

func (s *PlatformServiceServerImpl) ListDevelopers(ctx context.Context, req *authv1.ListDevelopersRequest) (*authv1.ListDevelopersResponse, error) {
	log.Printf("ListDevelopers request received")

	if _, err := s.requireSuperadmin(ctx, req.AccessToken); err != nil {
		log.Printf("ListDevelopers rejected: %v", err)
		return &authv1.ListDevelopersResponse{Success: false, Message: "Superadmin access required"}, nil
	}

	devs, err := s.repo.ListDevelopers(ctx)
	if err != nil {
		log.Printf("ListDevelopers: listing failed: %v", err)
		return &authv1.ListDevelopersResponse{Success: false, Message: "Internal server error"}, nil
	}

	// Resolve org names in one pass.
	orgNames := map[string]string{}
	if orgs, err := s.repo.ListOrganizations(ctx); err == nil {
		for _, o := range orgs {
			orgNames[o.OrgID] = o.Name
		}
	}

	out := make([]*authv1.DeveloperInfo, 0, len(devs))
	for _, d := range devs {
		out = append(out, &authv1.DeveloperInfo{
			DeveloperId: d.DeveloperID,
			Email:       d.Email,
			OrgId:       d.OrgID,
			OrgName:     orgNames[d.OrgID],
			CreatedAt:   timestamppb.New(d.CreatedAt),
		})
	}
	return &authv1.ListDevelopersResponse{Success: true, Message: "Developers retrieved successfully", Developers: out}, nil
}

func (s *PlatformServiceServerImpl) GetPlatformMetrics(ctx context.Context, req *authv1.GetPlatformMetricsRequest) (*authv1.GetPlatformMetricsResponse, error) {
	log.Printf("GetPlatformMetrics request received")

	if _, err := s.requireSuperadmin(ctx, req.AccessToken); err != nil {
		log.Printf("GetPlatformMetrics rejected: %v", err)
		return &authv1.GetPlatformMetricsResponse{Success: false, Message: "Superadmin access required"}, nil
	}

	resp := &authv1.GetPlatformMetricsResponse{Success: true, Message: "Metrics retrieved successfully"}
	for _, c := range []struct {
		dst   *int64
		count func(context.Context) (int64, error)
	}{
		{&resp.Developers, s.repo.CountDevelopers},
		{&resp.Orgs, s.repo.CountOrganizations},
		{&resp.Apps, s.repo.CountClients},
		{&resp.Users, s.repo.CountUsers},
		{&resp.ActiveSessions, s.repo.CountActiveSessions},
		{&resp.Tuples, s.repo.CountTuples},
	} {
		n, err := c.count(ctx)
		if err != nil {
			log.Printf("GetPlatformMetrics: count failed: %v", err)
			return &authv1.GetPlatformMetricsResponse{Success: false, Message: "Internal server error"}, nil
		}
		*c.dst = n
	}
	return resp, nil
}
