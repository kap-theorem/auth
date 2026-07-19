# kaplabs IAM Platform — Design Spec

**Date:** 2026-07-18
**Status:** Approved (brainstorm with abhira0)
**Repo:** `kaplabs/auth`

## 1. Purpose

Evolve the existing gRPC auth service into a production-grade, multi-tenant
IAM platform (AuthN + AuthZ) that any app can integrate — starting with
wordskali (currently HTTP Basic + SQLite) and dsapanicle. Internal-first
deployment, but the design must not preclude public exposure.

## 2. System overview

One Go service, one MySQL database, one gRPC API (plus a JSON/REST gateway
for the browser console). Three layers:

```
┌──────────────────────────────────────────────────┐
│         gRPC API (TLS) + grpc-gateway REST       │
├──────────────┬───────────────────┬───────────────┤
│    AuthN     │   Tenant mgmt     │     AuthZ     │
│ login/tokens │ orgs, devs, apps  │ tuples+check  │
├──────────────┴───────────────────┴───────────────┤
│                  MySQL (GORM)                    │
└──────────────────────────────────────────────────┘
```

Delivered in three phases, each shippable:

- **Phase 1 — AuthN hardening** (prerequisite for everything)
- **Phase 2 — Orgs & developer self-service** (+ web console)
- **Phase 3 — Authorization engine**

## 3. Entities

| Entity | Purpose | Key fields |
|---|---|---|
| **Organization** | Top-level owner. Personal org auto-created per developer (GitHub-style). | `org_id`, `name` |
| **Developer** | Platform-level account that builds on the service. Belongs to an org. Authenticates via a built-in "platform" client (we dogfood our own auth). | `developer_id`, `email`, password hash, `org_id` |
| **Client (app)** | A registered application; the tenant boundary. | `client_id`, bcrypt-hashed `client_secret`, `org_id`, `identity_scope: app\|org` |
| **User** | End user, scoped to either one app or one org (SSO). | `user_id`, `email`, password hash, `scope_type: app\|org`, `scope_id` |
| **Session** | One logged-in device. Multi-session per user. | `session_id` (PK), `user_id`, `client_id`, bcrypt-hashed refresh token, `expires_at`, `user_agent` |
| **AuthzModel** | Per-app authorization vocabulary: resource types, relations, implication rules. Versioned JSON. | `client_id`, `model_json`, `version` |
| **RelationTuple** | One access fact: subject has relation on object. Optional ABAC condition, optional explicit deny. | see §6 |

Relationships: Org owns Developers and Clients; Client has an AuthzModel and
(if `identity_scope=app`) its own Users; org-scoped Users are shared by all
of the org's `identity_scope=org` apps; Users have many Sessions; tuples
reference users/objects by name within a client scope.

**Identity scoping / SSO:** `identity_scope` is a per-app flag.
`app` (default) = fully isolated user base. `org` = login resolves against
the org's shared user pool → one account works across all the org's apps
(single sign-on at the credential/identity level). Sessions remain per
user+app. Authz tuples remain per-app in both modes — SSO shares *who you
are*, never *what you can do*.

**Uniqueness:** `UNIQUE(email, scope_type, scope_id)` — replaces the current
(broken) global email uniqueness.

## 4. Token design

- **JWT access token (RS256).** Claims: `sub` (user_id), `client_id`,
  `session_id`, `iat`, `exp`, `iss`. **No refresh token or username in the
  payload.** 30-minute lifetime.
- Private key held by the service; public keys served via `GetJWKS`, keyed
  by `kid` for rotation. Apps may verify tokens locally (no round-trip).
- **Refresh token:** opaque 256-bit random, stored bcrypt-hashed on its
  session row, rotated on every refresh. **Reuse of a rotated token =
  theft signal → revoke the whole session.** 7-day lifetime, sliding.
- `ValidateToken` remains for revocation-aware validation (checks the
  session row exists) and can optionally return the user's app-level roles.

## 5. AuthN API (Phase 1)

All RPCs require `client_id` + `client_secret`, verified against the bcrypt
hash in constant time.

- `RegisterUser`, `GetToken` (login), `ValidateToken`, `RefreshToken`,
  `RevokeToken` (logout single session)
- New: `LogoutAllSessions`, `GetUserSessions`, `RevokeSession` (by session_id)
- `ChangeUserPassword` (validates token + current password; enforces the
  same ≥8-char policy as registration; invalidates all other sessions)
- `RegisterClient` / `ChangeClientSecret`: gated by static `ADMIN_SECRET`
  env var until Phase 2 replaces them with developer-authenticated CreateApp.

**Rate limiting** (gRPC interceptor, in-memory token bucket; Redis only if
the service is ever replicated):
- Login: 5 attempts / email / client / 15 min
- Registration: 10 / client / hour
- Global per-client request ceiling (default 1000/min)

**Fixes rolled into Phase 1** (from the 2026-07-18 security review):
plaintext client secrets → bcrypt; refresh token in JWT payload → session_id;
unauthenticated RegisterClient → ADMIN_SECRET; global email uniqueness →
scoped; `email` vs `email_id` column mismatch in `GetUserByEmail`; single
session per user+client → multi-session; HS256 → RS256; no PII (emails) in
logs; TLS on the listener (cert paths via env; plaintext allowed only when
`ENV=dev`).

## 6. Authorization engine (Phase 3)

### Model
Apps declare their own vocabulary (the platform has no built-in roles):

```
app:      roles = admin, moderator, member
problem:  relations = author, editor, viewer
          author implies editor; editor implies viewer
```

Stored as versioned JSON in `authz_models`. Checks evaluate against the
latest version.

### Tuple store

```sql
CREATE TABLE relation_tuples (
  client_id      VARCHAR(36)  NOT NULL,
  object_type    VARCHAR(64)  NOT NULL,
  object_id      VARCHAR(128) NOT NULL,
  relation       VARCHAR(64)  NOT NULL,
  subject_type   VARCHAR(64)  NOT NULL,  -- 'user' | 'role'
  subject_id     VARCHAR(128) NOT NULL,
  effect         ENUM('allow','deny') NOT NULL DEFAULT 'allow',
  condition_expr TEXT NULL,              -- ABAC, usually NULL
  created_at     TIMESTAMP,
  PRIMARY KEY (client_id, object_type, object_id, relation,
               subject_type, subject_id, effect),
  INDEX idx_subject (client_id, subject_type, subject_id)
);
```

Resources are names chosen by the app (`problem:two-sum`); the platform
never stores the resources themselves. Rule of thumb: an object appears in
tuples only if access varies per instance; otherwise use an app-level role.

### Check resolution — `Check(client, subject, relation, object)`

1. **Deny pass first:** resolve deny tuples with the same expansion logic.
   Any match → DENY, stop. Deny always wins (AWS semantics). Deny matches
   only the exact relation named — it does not travel through implications.
2. **Allow pass:** expand the relation via the model's implication rules;
   direct tuple lookup; userset hop (subject_type='role' → recurse on role
   membership). Conditions on matched tuples are evaluated against request
   context; a failed condition means the tuple doesn't match.
3. No match → deny. **Default-deny, always** (least privilege).

Recursion depth-limited (20). Expected cost: 1–3 indexed queries per check.
Deny pass is one cheap EXISTS when a tenant has no deny tuples.

**Deliberately not built** (Zanzibar features we skip at this scale):
distributed caching, consistency zookies, cross-region replication. The
`Check()` API shape doesn't change if an engine like OpenFGA replaces the
internals later.

### AuthZ API
- `WriteAuthzModel` / `GetAuthzModel` (developer-authenticated, versioned)
- `WriteTuples` / `DeleteTuples` (batch, client-authenticated)
- `Check`, `ListObjects` ("everything user X can edit" — for UIs)

### Condition language (ABAC)
Minimal expression grammar over request context: comparisons, boolean
ops, `now()`, context vars (`ip`, custom key/values passed to Check).
Sandboxed, no side effects; parse errors fail closed (deny).

## 7. Tenant management API (Phase 2)

- `RegisterDeveloper`, `DeveloperLogin` — developers are users of a built-in
  `platform` client; personal org auto-created on signup.
- `CreateApp`, `ListApps`, `UpdateApp` (name, `identity_scope`),
  `RotateAppSecret`, `DeleteApp` — developer token + org ownership required.
- Secrets are shown once at creation/rotation, stored only as bcrypt hashes.
- Superadmin RPCs (require `Check(platform, caller, admin, app:platform)`):
  `ListAllOrgs`, `ListAllApps`, `SuspendClient`/`RestoreClient`,
  `ListDevelopers`, `GetPlatformMetrics`.
- Out of scope for v1: `InviteDeveloper` / multi-member orgs, billing,
  per-client password policies, open-vs-invite user signup policy.

## 8. Console (frontend) — auth.kaplabs.dev

Browser SPA served by the same deployment; talks JSON to the service via
grpc-gateway (additive; the gRPC surface is unchanged).

One login page serves **two audiences**, distinguished by platform-level
role, not separate apps:

- **App admins (developers):** see and manage only their own org's apps —
  app list, credentials, secret rotation, authz models, tuples.
- **Superadmins (platform operators):** additionally see all orgs/apps,
  can suspend clients, view platform-wide metrics, and manage developers.

Superadmin is not a schema flag — it is a relation tuple on the built-in
platform client (`app:platform admin user:<dev>`), evaluated by our own
Check API. The console asks `Check(platform, me, admin, app:platform)`
after login and renders the superadmin navigation only on allow; the
backend enforces the same check on every admin-only RPC (UI hiding is
convenience, never the security boundary). Seed superadmins via a bootstrap
env var (`SUPERADMIN_EMAILS`) on first run.

Phasing note: the superadmin check needs only a direct tuple lookup, so the
`relation_tuples` table and a minimal `Check` (no implications/usersets)
land with Phase 2; the full resolver arrives in Phase 3 without API change.

- **Stack:** React + Vite + TypeScript, no UI framework beyond a small
  component set; single-purpose console, not a design system.
- **Developer pages:** sign-up/login → app list → app detail (credentials,
  secret rotation with show-once modal, identity_scope toggle) → authz model
  editor (JSON with validation) → tuple browser (filter by object/subject,
  add/delete tuples, effect + condition fields) → a "Check tester" panel
  (run a Check against live data — the debugging tool every authz system
  needs).
- **Superadmin pages:** org/app directory with suspend/restore, developer
  directory, platform metrics (logins, checks, error rates).
- Auth: the console is itself a client app of the platform (`identity_scope:
  org`), using the same JWT flow — dogfooding.
- Until Phase 2/3 APIs exist, the console builds against a typed mock API
  layer with the same interface as the generated gateway client.

## 9. Error handling, logging

- Errors in response messages with `success=false`; credential failures are
  always generic ("invalid credentials" — never distinguish wrong-password
  from no-such-user). Detail only in server logs.
- No PII in logs: user_ids and client_ids, never emails or tokens.
- Structured request logging stays in the unary interceptor.

## 10. Testing

Load-bearing suites, in priority order:

1. **Authz resolver table tests** — tuple+model fixtures with expected Check
   results: implication chains, userset hops, deny-wins, deny-no-implication,
   conditions (pass/fail/parse-error→deny), depth limit. The most important
   test artifact in the project.
2. **Cross-tenant isolation** — per RPC: client A's credentials can never
   read or mutate client B's data; org-scoped vs app-scoped identity
   resolution.
3. **Token misuse** — expired, tampered, cross-client, revoked-session
   tokens; refresh rotation and reuse-detection (session revoked).
4. **Rate limiting** — lockout after threshold, window reset.

## 11. Known trade-offs (accepted)

- Single service/DB = shared availability and blast radius across tenants;
  mitigate with per-client rate limits and client-side check caching (TTL).
- Recursive resolution on MySQL: fine at current scale; denormalize/cache
  or swap in OpenFGA behind the same API if a tenant grows large.
- No zookie-style consistency: a Check racing a WriteTuples may briefly see
  old data.
- Homegrown resolver + condition evaluator = we own the correctness burden;
  paid back by the resolver test suite.

## 12. GCP/AWS terminology mapping (for readers with cloud-IAM background)

| GCP | AWS | This system |
|---|---|---|
| Member | Principal | Subject (`user:kush`) |
| Permission | Action | Relation (via model implications) |
| Role | Managed policy | Relation used as a userset (`role:moderator`) |
| Policy binding | Policy attachment | RelationTuple (one binding = one row) |
| IAM Condition | Condition block | `condition_expr` on a tuple |
| Deny policy | Explicit Deny | `effect='deny'` tuple (deny wins) |
