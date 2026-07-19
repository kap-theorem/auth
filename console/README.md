# kaplabs IAM console

Browser console for the multi-tenant IAM platform (design spec:
`docs/superpowers/specs/2026-07-18-iam-platform-design.md`, §8).
React + Vite + TypeScript; the only runtime dependency beyond React is
react-router. Plain CSS, no component framework.

## Run it

```sh
npm install
npm run dev      # http://localhost:5173
npm run build    # typecheck + production build
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

When the grpc-gateway JSON client exists, implement `ApiClient` with it and
swap the construction in `src/api/index.ts`; no page code changes.

## Layout

```
src/
  api/        types.ts (ApiClient interface), mock.ts (MockApiClient), index.ts
  auth.tsx    session context + post-login superadmin Check
  components/ Layout (shell/nav), SecretModal (show-once), Fact (tuple as text)
  pages/      Login, Apps, AppDetail (credentials / model / tuples / check tabs)
  pages/admin Directory, Developers, Metrics (superadmin only)
  styles.css  design tokens + all styling
```
