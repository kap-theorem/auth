# kaplabs IAM console

Browser console for the multi-tenant IAM platform (design spec:
`docs/superpowers/specs/2026-07-18-iam-platform-design.md`, §8).
React + Vite + TypeScript; the only runtime dependency beyond React is
react-router. Plain CSS, no component framework.

## Run it

```sh
npm install
npm run dev      # http://localhost:5173 (mock API by default)
npm run build    # typecheck + production build
```

## Run against the real backend

```sh
# backend must be serving /auth.v1.PlatformService/* on localhost:8081
VITE_API_MODE=real npm run dev
```

`VITE_API_MODE=real` selects `RealApiClient` (`src/api/real.ts`); anything
else (or unset) keeps the in-memory mock, so the console works before the
backend lands. `src/api/index.ts` is the single swap point, and the sidebar
footer shows which mode is active (`api: mock` / `api: real`).

The vite dev server proxies `/auth.v1` → `http://localhost:8081`
(vite.config.ts), so no CORS setup is needed in dev.

### Wire contract (what RealApiClient speaks)

- HTTP POST to `/auth.v1.PlatformService/{MethodName}` with JSON bodies in
  protojson style (camelCase fields).
- Requests carry `accessToken` in the body; the token (+ refresh token) is
  kept in sessionStorage.
- **Token refresh**: on a 401 / invalid-token response the client calls
  `DeveloperRefreshToken` once with the stored refresh token
  (`{refreshToken}` → `{success, message, accessToken, refreshToken}`),
  persists the rotated pair (the returned refresh token replaces the old
  one; replaying an old one revokes the session), and retries the original
  request once. Parallel requests share a single in-flight refresh. If the
  refresh fails, the session is cleared and "session expired, sign in
  again" is surfaced. The client also refreshes **proactively** ~60s before
  the access token's `exp` (JWTs expire in 30 min) via one timer, decoded
  from the token's base64url payload — reset on login/refresh, cleared on
  sign-out.
- Methods: RegisterDeveloper (then an automatic DeveloperLogin, since
  registration returns ids only), DeveloperLogin, CreateApp / ListApps /
  UpdateApp / RotateAppSecret / DeleteApp, WriteAuthzModel / GetAuthzModel,
  WriteTuples / DeleteTuples / ListTuples, Check, and superadmin
  ListAllOrgs / ListAllApps / SuspendClient / RestoreClient /
  ListDevelopers / GetPlatformMetrics (simple counts: developers, orgs,
  apps, users, activeSessions, tuples).
- Wire app shape `{clientId, name, orgId, identityScope, suspended,
  createdAt}` and tuple shape `{objectType, objectId, relation, subjectType,
  subjectId, effect, conditionExpr?}` are mapped to the internal snake_case
  types in `src/api/real.ts`; `suspended` ↔ `status`.
- `ListObjects` is not in the contract yet; `RealApiClient.listObjects`
  throws (no page calls it). The console's substring tuple filter is applied
  client-side on top of `ListTuples`.

## Users tab (app user management)

App detail → **Users** lists the app's end users (spec "App user
management"): debounced search on username/email, per-user session count,
created date, and active status. Users from an org-scoped app's shared pool
carry an `org` badge — deactivating one disables the identity across every
app in the org, and the UI warns before doing it.

Per-row actions:

- **Sessions** — expands the row to show the user's active sessions (user
  agent, started) with a "Revoke all sessions" button
  (`ListUserSessionsAdmin` / `RevokeUserSessionsAdmin`).
- **Deactivate / Reactivate** — `SetUserActive`; deactivation revokes all
  the user's sessions and blocks every login flow (confirm required, with
  the org-wide warning for org-scoped users).
- **Make/Remove admin** — convenience writer for the exact tuple
  `app:<client_id> admin user:<user_id>` via WriteTuples/DeleteTuples;
  current state is read from ListTuples on load.

## Hosted account page

Public iframe-friendly page where an app's end user manages their own
account — no console session, no client secret in the browser (spec
"Hosted account page"). Available only for apps with a non-empty hosted
redirect whitelist (the hosted opt-in); the app must not be suspended.

- **URL:** `/client/{client_id}/user/profile`
- Talks directly to the real gateway (`HostedGetProfile`,
  `HostedChangePassword`, `HostedRevokeSession`, `HostedLogoutAll`),
  regardless of `VITE_API_MODE` — same as the hosted login page.
- Sections: profile card (username, email), active sessions (user agent,
  created, `current` badge, per-row revoke), change password (client-side
  ≥8 check; on success other devices are signed out, the current session
  stays), and "Sign out everywhere".

### Token acquisition (priority order)

1. **URL fragment:** open the page as
   `/client/{client_id}/user/profile#access_token=<jwt>`. The fragment is
   scrubbed from the address bar after being read.
2. **postMessage handshake** (iframe embedding):
   - on mount the page posts `{type: "PROFILE_READY"}` to `window.parent`
     (targetOrigin `"*"` — carries no secret; the handshake exists to learn
     the parent's origin);
   - the embedding app replies with
     `{type: "AUTH_TOKEN", access_token: "<jwt>"}` targeted at the iframe;
   - the page validates `event.origin` is an http(s) origin and remembers
     it — all later messages back to the parent are scoped to that origin.
   - No token within ~2s → a friendly "not signed in" state.

### LOGGED_OUT event

After a successful "Sign out everywhere" (`HostedLogoutAll` revokes every
session for this client, including the current one), the page posts
`{type: "LOGGED_OUT"}` to the parent (verified handshake origin when
available) so the host app can clear its own cookie and redirect.

Embedding app snippet:

```html
<iframe id="account" src="https://auth.kaplabs.dev/client/CLIENT_ID/user/profile"></iframe>
<script>
  const frame = document.getElementById("account");
  window.addEventListener("message", (e) => {
    if (e.data?.type === "PROFILE_READY") {
      frame.contentWindow.postMessage(
        { type: "AUTH_TOKEN", access_token: myAccessToken },
        new URL(frame.src).origin
      );
    } else if (e.data?.type === "LOGGED_OUT") {
      // clear your session cookie, then redirect to your login page
    }
  });
</script>
```

## Demo logins (mock data)

| Audience | Email | Password |
|---|---|---|
| Superadmin (platform operator) | `root@kaplabs.dev` | `superadmin` |
| Developer (app admin) | `ada@example.dev` | `developer` |

One login page serves both audiences. After login the console calls
`Check(platform, me, admin, app:platform)`; the mock seeds the tuple
`app:platform admin user:dev_root`, so only the superadmin account gets the
Platform navigation (orgs & apps directory with suspend/restore, developer
directory, metrics). You can also register a fresh developer account from the
login page.

The developer account owns two seeded apps — try the **dsapanicle** app: it has
a seeded authz model (`author → editor → viewer` implications), tuples with a
role userset (`role:moderator`), an explicit deny (`user:banned`), and a
conditional tuple (`user:contractor` viewer on `problem:binary-search` only
when context `{"env": "staging"}`). The Check tester exercises all of them,
and shows the resolver's `reason` string under the ALLOWED/DENIED verdict.

Condition grammar (tuple `condition_expr`): comparisons `==`, `!=`, `<`, `>`,
joined with `&&` / `||`; `now()`, context keys as identifiers, string literals
in double quotes — e.g. `env == "staging" && now() < "2026-08-01T00:00:00Z"`.
(The in-memory mock evaluates only the `key == "value"` / `key != "value"`
`&&` subset and fails closed on the rest.)

## What's mocked

Everything behind `src/api/types.ts` (`ApiClient`). `src/api/mock.ts` is an
in-memory implementation with seeded orgs/developers/apps/models/tuples and a
scaled-down Check resolver (deny pass first, implication expansion from the
model JSON, role userset hops, `key == "value"` conditions that fail closed,
depth limit 20). Data resets on reload; the signed-in developer survives reload
via sessionStorage.

The interface mirrors the spec's RPCs: DeveloperLogin, RegisterDeveloper,
CreateApp/ListApps/UpdateApp/RotateAppSecret/DeleteApp, WriteAuthzModel/
GetAuthzModel, WriteTuples/DeleteTuples/Check/ListObjects, user management
ListAppUsers/ListUserSessionsAdmin/RevokeUserSessionsAdmin/SetUserActive
(mock seeds a few wordskali users — one deactivated, one app admin — and an
org-scoped user under dsapanicle), plus superadmin
ListAllOrgs/ListAllApps/SuspendClient/RestoreClient/ListDevelopers/
GetPlatformMetrics. `listTuples` is a console read helper — the gateway will
need an equivalent ReadTuples RPC for the tuple browser.

The real client (`src/api/real.ts`) implements the same `ApiClient`
interface; `src/api/index.ts` picks one at startup based on
`VITE_API_MODE`. No page code changes between modes.

## Layout

```
src/
  api/        types.ts (ApiClient interface), mock.ts (MockApiClient),
              real.ts (RealApiClient, HTTP JSON), index.ts (mode selection)
  auth.tsx    session context + post-login superadmin Check
  components/ Layout (shell/nav), SecretModal (show-once), Fact (tuple as text)
  pages/      Login, Apps, AppDetail (credentials / model / tuples / check tabs)
  pages/admin Directory, Developers, Metrics (superadmin only)
  styles.css  design tokens + all styling
```
