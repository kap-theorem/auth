package models

import (
	"time"

	"gorm.io/gorm"
)

// Scope / identity-scope values shared by Client.IdentityScope and
// User.ScopeType.
const (
	ScopeApp = "app"
	ScopeOrg = "org"
)

// Relation tuple effects.
const (
	EffectAllow = "allow"
	EffectDeny  = "deny"
)

// Organization is the top-level owner of developers and clients. A personal
// org is auto-created for every developer on signup (GitHub-style).
type Organization struct {
	OrgID     string         `gorm:"column:org_id;primaryKey;size:36" json:"org_id"`
	Name      string         `gorm:"size:100;not null" json:"name"`
	CreatedAt time.Time      `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time      `gorm:"autoUpdateTime" json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

// Developer links a platform-client user to their personal org. The
// developer's credentials live on the User row (scope_type='org',
// scope_id=<platform org>) — we dogfood our own auth. DeveloperID equals
// that user's user_id.
type Developer struct {
	DeveloperID string         `gorm:"column:developer_id;primaryKey;size:36" json:"developer_id"`
	Email       string         `gorm:"column:email;size:255;not null;uniqueIndex" json:"email"`
	OrgID       string         `gorm:"column:org_id;size:36;not null;index" json:"org_id"`
	CreatedAt   time.Time      `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt   time.Time      `gorm:"autoUpdateTime" json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`
}

type Client struct {
	ClientID   string `gorm:"column:client_id;primaryKey;size:36" json:"client_id"`
	ClientName string `gorm:"size:100;not null" json:"client_name"`
	// Bcrypt hash of the client secret. The plaintext secret is shown once
	// at registration/rotation and never stored.
	ClientSecretHash string `gorm:"column:client_secret;size:255;not null" json:"-"`
	// Owning organization.
	OrgID string `gorm:"column:org_id;size:36;index" json:"org_id"`
	// 'app' (default) = isolated user base; 'org' = login resolves against
	// the org's shared user pool (SSO at the identity level).
	IdentityScope string `gorm:"column:identity_scope;size:8;not null;default:app" json:"identity_scope"`
	// Suspended clients fail all RPC authentication.
	Suspended bool           `gorm:"column:suspended;not null;default:false" json:"suspended"`
	CreatedAt time.Time      `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time      `gorm:"autoUpdateTime" json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

type User struct {
	UserID   string `gorm:"column:user_id;primaryKey;size:36" json:"user_id"`
	UserName string `gorm:"column:user_name;size:100;not null" json:"username"`
	// Email uniqueness is scoped: UNIQUE(email_id, scope_type, scope_id).
	Email    string `gorm:"column:email_id;size:255;not null;uniqueIndex:idx_users_email_scope" json:"email"`
	Password string `gorm:"size:255;not null" json:"-"`
	// ScopeType/ScopeID replace the former client_id column:
	//   scope_type='app', scope_id=<client_id>  — app-scoped user base
	//   scope_type='org', scope_id=<org_id>     — org-shared user pool (SSO)
	ScopeType string         `gorm:"column:scope_type;size:8;not null;uniqueIndex:idx_users_email_scope" json:"scope_type"`
	ScopeID   string         `gorm:"column:scope_id;size:36;not null;index;uniqueIndex:idx_users_email_scope" json:"scope_id"`
	CreatedAt time.Time      `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time      `gorm:"autoUpdateTime" json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

// Session represents one logged-in device. A user may hold many sessions
// per client (multi-session).
type Session struct {
	SessionID string `gorm:"column:session_id;primaryKey;size:36" json:"session_id"`
	UserID    string `gorm:"column:user_id;size:36;not null;index" json:"user_id"`
	ClientID  string `gorm:"column:client_id;size:36;not null;index" json:"client_id"`
	// Bcrypt hash of the secret half of the opaque refresh token
	// ("<session_id>.<secret>"). Rotated on every refresh.
	RefreshTokenHash string         `gorm:"column:refresh_token_hash;size:255;not null" json:"-"`
	UserAgent        string         `gorm:"size:500" json:"user_agent"`
	ExpiresAt        time.Time      `gorm:"not null" json:"expires_at"`
	CreatedAt        time.Time      `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt        time.Time      `gorm:"autoUpdateTime" json:"updated_at"`
	DeletedAt        gorm.DeletedAt `gorm:"index" json:"-"`
}

// RelationTuple is one access fact: subject has relation on object within a
// client scope (design spec §6). Optional explicit deny and ABAC condition.
type RelationTuple struct {
	ClientID    string `gorm:"column:client_id;primaryKey;size:36;index:idx_relation_tuples_subject,priority:1" json:"client_id"`
	ObjectType  string `gorm:"column:object_type;primaryKey;size:64" json:"object_type"`
	ObjectID    string `gorm:"column:object_id;primaryKey;size:128" json:"object_id"`
	Relation    string `gorm:"column:relation;primaryKey;size:64" json:"relation"`
	SubjectType string `gorm:"column:subject_type;primaryKey;size:64;index:idx_relation_tuples_subject,priority:2" json:"subject_type"` // 'user' | 'role'
	SubjectID   string `gorm:"column:subject_id;primaryKey;size:128;index:idx_relation_tuples_subject,priority:3" json:"subject_id"`
	Effect      string `gorm:"column:effect;primaryKey;size:8;not null;default:allow" json:"effect"` // 'allow' | 'deny'
	// ABAC condition; usually empty. Evaluated by the Phase 3 resolver —
	// the Phase 2 minimal Check fails closed on conditioned tuples.
	ConditionExpr string    `gorm:"column:condition_expr;type:text" json:"condition_expr"`
	CreatedAt     time.Time `gorm:"autoCreateTime" json:"created_at"`
}

func (RelationTuple) TableName() string { return "relation_tuples" }

// AuthzModel is a per-app authorization vocabulary, stored as versioned
// JSON. Checks evaluate against the latest version.
type AuthzModel struct {
	ClientID  string    `gorm:"column:client_id;primaryKey;size:36" json:"client_id"`
	Version   int32     `gorm:"column:version;primaryKey;autoIncrement:false" json:"version"`
	ModelJSON string    `gorm:"column:model_json;type:text;not null" json:"model_json"`
	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
}

func (AuthzModel) TableName() string { return "authz_models" }

func GetAllModels() []any {
	return []any{
		&Organization{},  // Orgs first (parent of clients and developers)
		&Client{},        // Clients (reference orgs)
		&Developer{},     // Developers (reference orgs)
		&User{},          // Users (scoped to an app or an org)
		&Session{},       // Sessions (reference users and clients)
		&RelationTuple{}, // Authz tuples (scoped by client_id)
		&AuthzModel{},    // Versioned per-app authz models
	}
}
