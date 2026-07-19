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
  kept in sessionStorage. There is no developer token refresh RPC in the
  contract (AuthService `RefreshToken` needs a client secret the console
  never holds), so on a 401 / invalid-token response the client clears the
  session and surfaces "session expired, sign in again".
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
when context `{"env": "staging"}`). The Check tester exercises all of them.

## What's mocked

Everything behind `src/api/types.ts` (`ApiClient`). `src/api/mock.ts` is an
in-memory implementation with seeded orgs/developers/apps/models/tuples and a
scaled-down Check resolver (deny pass first, implication expansion from the
model JSON, role userset hops, `key == "value"` conditions that fail closed,
depth limit 20). Data resets on reload; the signed-in developer survives reload
via sessionStorage.

The interface mirrors the spec's RPCs: DeveloperLogin, RegisterDeveloper,
CreateApp/ListApps/UpdateApp/RotateAppSecret/DeleteApp, WriteAuthzModel/
GetAuthzModel, WriteTuples/DeleteTuples/Check/ListObjects, plus superadmin
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
