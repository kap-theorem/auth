# Hosted app features: demo login, login-identifier, public signup, self-edit + locks

Date: 2026-07-19
Status: Design approved, pre-implementation
Repo: `git_repos/kaplabs/auth`

## Background

`wordskali` migrated off the old auth to this hardened platform. The old
system had four end-user/app features this platform does **not** yet have:
one-click demo login, a login-identifier mode (username vs email), public
self-signup with an invite-only toggle, and admin/self user editing with
per-field locks. This doc designs all four as additions to the auth server so
every hosted app gets them.

The authz/IAM layer (relation tuples, roles, ABAC, `Check`) is unrelated to
these — it answers "can user X do Y?". It is used only in feature 1 to scope
what a demo account may do. The other three are auth product/UI features, not
authorization.

## Shared plumbing (how a change lands end-to-end)

Established pipeline (evidence: prior Explore of the repo):

- **Proto:** edit `proto/auth/v1/{auth,platform}.proto`, then `make proto`
  (protoc; generated `*.pb.go` are committed).
- **HTTP gateway:** generic reflective shim in
  `internal/httpgateway/gateway.go` — new RPCs auto-register as
  `POST /auth.v1.<Service>/<Method>`; **no manual wiring**.
- **Model:** GORM structs in `pkg/models/model.go`; schema via `AutoMigrate`
  at boot (`internal/database/sqlConnection.go:128`). No SQL migration files.
  Ad-hoc data migrations run before AutoMigrate, GORM-`Migrator()`-guarded
  (pattern: `migrateUserScoping()` at `sqlConnection.go:146`).
- **App-field pipeline:** `message App` (`platform.proto`) +
  `UpdateAppRequest` (use `optional`) → `models.Client` column → `appToProto`
  + `UpdateApp` (`pkg/service/platform_service.go`) → console `real.ts`
  (`WireApp`/`mapApp`/`updateApp`) + a control in `AppDetail.tsx`.
- **Hosted pages:** `console/src/pages/HostedLogin.tsx` and
  `HostedProfile.tsx` POST protojson directly to
  `/auth.v1.PlatformService/*` (local `post()` helper, bypasses mock/real
  switch). Token delivery: iframe `postMessage({type:'AUTH_SUCCESS'})` or
  fragment redirect.
- **Tests:** Go table tests in `pkg/service/*_test.go` using in-memory SQLite
  (`newTestDB` + `seedClient`/`seedUser`). Prod is MySQL — verify any new
  index against MySQL too. Console has **no** test harness (out of scope;
  don't add one).

Trust model for secret-free hosted RPCs: mirror `HostedLogin`
(`platform_service.go:790`) — gated by `hostedEnabledClient` + exact
whitelisted `redirect_uri`, no `client_secret`.

## Feature 1 — Demo user login

**Goal:** one-click "Login as Demo User" on the hosted login page.

- **Model (`models.Client`):** `demo_enabled bool`, `demo_email string`,
  `demo_password string` (server-side only — never returned to a browser).
- **RPC:** new secret-free `PlatformService.HostedDemoLogin(client_id,
  redirect_uri)`, gated by `hostedEnabledClient` + `demo_enabled`. Internally
  performs the stored-credential login (reuse `issueLoginTokens`) and returns
  the same token payload as `HostedLogin`. The stored demo password is used
  server-side only.
- **Public info:** `GetAppPublicInfo` gains a `demo_enabled` bool so the login
  page knows whether to render the button. It must **not** return
  `demo_email`/`demo_password`.
- **UI (`HostedLogin.tsx`):** render "Login as Demo User" below the form when
  `demo_enabled`; on click call `HostedDemoLogin`, then the existing token
  delivery path.
- **Console (`AppDetail.tsx`):** demo card — enable toggle + email + password
  fields, persisted via `UpdateApp`.
- **IAM tie-in:** the demo account is an ordinary user; restrict its
  capabilities with authz tuples (e.g. deny writes). No special code.
- **Tests:** demo login disabled → button absent / RPC rejects; enabled →
  returns valid token; demo creds never present in `GetAppPublicInfo` output.

## Feature 2 — Login identifier mode

**Goal:** admin chooses, per app, whether users log in with username, email,
or either.

- **Model (`models.Client`):** `login_identifier string` enum
  `username_or_email` (default) | `email_only` | `username_only`.
- **Username uniqueness (decision: enforce + migrate):**
  - `user_name` currently has **no unique index** and duplicates may exist
    (`model.go:98`). Add a composite unique index over
    `(user_name, scope_type, scope_id)` mirroring `idx_users_email_scope`.
  - **Reconciliation migration** (ad-hoc Go func before AutoMigrate, pattern
    `migrateUserScoping()`): within each scope, find duplicate `user_name`s
    and rename all but the oldest by suffixing `-2`, `-3`, … (checking the
    suffixed name is itself free). Guard with
    `Migrator().HasIndex(&User{}, "idx_users_username_scope")` so it runs
    once. Log every rename. Only after reconciliation does AutoMigrate add the
    unique index (adding it on dup data would fail).
- **Backend:** `issueLoginTokens` gains a lookup path: resolve the submitted
  identifier by email and/or username per the app's mode, within the client's
  scope. Add `GetUserByUsername(username, scopeType, scopeID)` to
  `auth_repository.go`. Enforce mode: reject an email submitted to a
  `username_only` app and vice-versa.
- **Registration:** enforce username uniqueness at `RegisterUser` /
  `HostedRegister` (reject duplicate within scope) so the index invariant
  holds going forward.
- **UI (`HostedLogin.tsx`):** the identifier `<input>` label/placeholder and
  client validation follow `login_identifier` (from `GetAppPublicInfo`).
- **Console:** a three-way selector in `AppDetail.tsx`.
- **Tests:** each mode accepts the right identifier and rejects the wrong one;
  reconciliation renames dups and preserves the oldest; duplicate-username
  registration rejected.

## Feature 3 — Public signup + invite-only toggle

**Goal:** optional self-serve signup; invite-only (admin-provisioned) by
default.

- **Model (`models.Client`):** `public_signup bool` (default `false` =
  invite-only).
- **RPC:** new secret-free `PlatformService.HostedRegister(client_id, email,
  username, password, redirect_uri)`, gated by `hostedEnabledClient` +
  `public_signup`. Reuse `validateUserRegistration`, `IsEmailExists`,
  `CreateUser`. Trust anchored on the redirect-URI whitelist (same as
  `HostedLogin`); **no `client_secret`**.
- **Rate limiting (flag):** the register limiter is keyed only by `client_id`
  (`ratelimit.go:82,121`), so anonymous signup lets one app's visitors exhaust
  the shared bucket. Add a **per-IP** key for the hosted-register path (client
  IP from the gateway). Keep the existing per-client ceiling as a backstop.
- **UI (`HostedLogin.tsx`):** show a "Sign up" link/form only when
  `public_signup` (from `GetAppPublicInfo`); on success, deliver token like
  login (auto-login the new user) or route back to login — default: auto-login.
- **Invite-only path:** default state; admin creates users in the console
  (existing authenticated `RegisterUser`). Console needs a "create user"
  action if not already present.
- **Console:** `public_signup` toggle in `AppDetail.tsx`.
- **Tests:** signup disabled → RPC rejects, no link; enabled → creates user +
  returns token; duplicate email rejected; per-IP rate limit trips.

## Feature 4 — Self-edit + admin Edit-User + field locks

**Goal:** users edit their own username/email; admins edit any user and lock
individual fields from self-edit.

- **Model (`models.User`, per-user, matching the old UI):**
  `lock_username bool`, `lock_email bool`, `lock_password bool`.
- **RPCs:**
  - `PlatformService.HostedUpdateProfile(access_token, username?, email?)` —
    self-edit, auth via `authenticateHostedUser` (like
    `HostedChangePassword`). Rejects a field whose lock is set. Email change
    re-checked against `idx_users_email_scope`; username against the new
    username unique index (feature 2).
  - Admin `UpdateUser(user_id, username?, email?, password?, lock_*?)` —
    extend the existing user-admin surface (`SetUserActive`,
    `user_admin_test.go`). Admins bypass locks and set them.
  - `HostedChangePassword` gains a `lock_password` check.
- **UI:**
  - `HostedProfile.tsx`: make username/email editable (currently read-only,
    `:281-283`); hide/disable a field when its lock is set.
  - Console: an "Edit User" dialog (username/email/new-password + three lock
    toggles), matching the old screenshot.
- **Tests:** locked field rejected on self-edit but allowed for admin; email
  uniqueness enforced on edit; lock toggles persist.

## Build order

Each feature is its own implementation plan, built and shipped in order:

1. **Demo login** — smallest, self-contained, highest daily value (wordskali).
2. **Login identifier** — includes the username-uniqueness migration.
3. **Public signup + invite-only** — depends on username uniqueness (f2) for
   the registration invariant.
4. **Self-edit + admin edit + locks** — largest; uses the username index (f2)
   and email index for edit validation.

## Non-goals

- No forgot-password / reset flow (absent today; not requested).
- No console test harness (greenfield; out of scope).
- No changes to `wordskali` beyond consuming these once shipped (the login
  page already renders the hosted UI; new buttons/links appear from the auth
  side).
- No wildcard redirect URIs; exact-match whitelist unchanged.
