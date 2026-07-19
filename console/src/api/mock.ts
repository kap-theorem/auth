// In-memory mock of the platform API. Same interface as the future
// grpc-gateway client; swap the export in src/api/index.ts when it lands.
//
// Implements a scaled-down version of the spec's Check resolution (§6):
// deny pass first (exact relation, deny wins), then allow pass with
// implication expansion and role (userset) hops, conditions fail closed.

import {
  ApiClient,
  ApiError,
  App,
  AppUser,
  AuthzModel,
  CheckResult,
  Developer,
  DeveloperSession,
  IdentityScope,
  LoginIdentifier,
  Organization,
  PLATFORM_CLIENT_ID,
  PlatformMetrics,
  RelationTuple,
  TupleFilter,
  TupleKey,
  UserScope,
  UserSession,
} from "./types";

// ---------------------------------------------------------------- seed data

interface DevRecord extends Developer {
  password: string;
}

const now = () => new Date().toISOString();
const LATENCY_MS = 180;

const orgs: Organization[] = [
  { org_id: "org_platform", name: "kaplabs (platform)", created_at: "2026-01-05T09:00:00Z" },
  { org_id: "org_ada", name: "ada — personal org", created_at: "2026-02-11T14:20:00Z" },
  { org_id: "org_meridian", name: "meridian labs", created_at: "2026-03-02T10:05:00Z" },
];

const developers: DevRecord[] = [
  {
    developer_id: "dev_root",
    email: "root@kaplabs.dev",
    password: "superadmin",
    org_id: "org_platform",
    created_at: "2026-01-05T09:00:00Z",
  },
  {
    developer_id: "dev_ada",
    email: "ada@example.dev",
    password: "developer",
    org_id: "org_ada",
    created_at: "2026-02-11T14:20:00Z",
  },
  {
    developer_id: "dev_noor",
    email: "noor@meridian.dev",
    password: "developer",
    org_id: "org_meridian",
    created_at: "2026-03-02T10:05:00Z",
  },
];

const apps: App[] = [
  {
    client_id: PLATFORM_CLIENT_ID,
    org_id: "org_platform",
    name: "platform",
    identity_scope: "org",
    status: "active",
    created_at: "2026-01-05T09:00:00Z",
    redirect_uris: [],
    demo_enabled: false,
    login_identifier: "username_or_email",
    public_signup: false,
  },
  {
    client_id: "client_wordskali",
    org_id: "org_ada",
    name: "wordskali",
    identity_scope: "app",
    status: "active",
    created_at: "2026-02-11T14:25:00Z",
    redirect_uris: ["https://wordskali.example.dev/auth/callback"],
    demo_enabled: true,
    login_identifier: "username_or_email",
    public_signup: true,
  },
  {
    client_id: "client_dsapanicle",
    org_id: "org_ada",
    name: "dsapanicle",
    identity_scope: "org",
    status: "active",
    created_at: "2026-04-19T08:12:00Z",
    redirect_uris: [],
    demo_enabled: false,
    login_identifier: "email_only",
    public_signup: false,
  },
  {
    client_id: "client_helioscan",
    org_id: "org_meridian",
    name: "helioscan",
    identity_scope: "app",
    status: "suspended",
    created_at: "2026-03-02T10:10:00Z",
    redirect_uris: [],
    demo_enabled: false,
    login_identifier: "username_or_email",
    public_signup: false,
  },
];

// model_json shape: { "types": { "<object_type>": { "relations": {
//   "<relation>": ["<relations that imply it>"] } } } }
const dsapanicleModel = {
  types: {
    app: { relations: { admin: [], moderator: ["admin"], member: ["moderator"] } },
    problem: { relations: { author: [], editor: ["author"], viewer: ["editor"] } },
  },
};

const models = new Map<string, AuthzModel>([
  [
    "client_dsapanicle",
    {
      client_id: "client_dsapanicle",
      model_json: JSON.stringify(dsapanicleModel, null, 2),
      version: 3,
      updated_at: "2026-06-30T17:40:00Z",
    },
  ],
]);

const tuples = new Map<string, RelationTuple[]>([
  [
    PLATFORM_CLIENT_ID,
    [
      // Superadmin is a relation tuple, not a schema flag (spec §8).
      t("app", "platform", "admin", "user", "dev_root"),
    ],
  ],
  [
    "client_dsapanicle",
    [
      t("problem", "two-sum", "author", "user", "kush"),
      t("problem", "two-sum", "editor", "role", "moderator"),
      t("problem", "binary-search", "author", "user", "mina"),
      t("problem", "binary-search", "viewer", "user", "contractor", "allow", 'env == "staging"'),
      t("role", "moderator", "member", "user", "mina"),
      t("app", "dsapanicle", "moderator", "user", "mina"),
      t("problem", "two-sum", "viewer", "user", "banned", "deny"),
    ],
  ],
  [
    "client_wordskali",
    [
      t("board", "daily", "owner", "user", "ada"),
      // u_kush is an app admin — the Users tab's Admin toggle reads/writes
      // exactly this tuple shape.
      t("app", "client_wordskali", "admin", "user", "u_kush"),
    ],
  ],
]);

function t(
  object_type: string,
  object_id: string,
  relation: string,
  subject_type: "user" | "role",
  subject_id: string,
  effect: "allow" | "deny" = "allow",
  condition_expr?: string
): RelationTuple {
  return {
    object_type,
    object_id,
    relation,
    subject_type,
    subject_id,
    effect,
    condition_expr,
    created_at: "2026-06-01T12:00:00Z",
  };
}

// End users, scoped like the backend: scope 'app'/scope_id=client_id is an
// app's own user base; scope 'org'/scope_id=org_id is the org's shared pool
// (surfaced by every org-scoped app of that org).
interface MockEndUser {
  user_id: string;
  username: string;
  email: string;
  created_at: string;
  active: boolean;
  scope_type: UserScope;
  scope_id: string;
  lock_username?: boolean;
  lock_email?: boolean;
  lock_password?: boolean;
}

const endUsers: MockEndUser[] = [
  // wordskali's own users. u_kush also holds the app-admin tuple (below).
  {
    user_id: "u_kush",
    username: "kush",
    email: "kush@example.com",
    created_at: "2026-03-14T09:30:00Z",
    active: true,
    scope_type: "app",
    scope_id: "client_wordskali",
  },
  {
    user_id: "u_mina",
    username: "mina",
    email: "mina@example.com",
    created_at: "2026-04-02T16:45:00Z",
    active: true,
    scope_type: "app",
    scope_id: "client_wordskali",
  },
  {
    user_id: "u_banned",
    username: "banned",
    email: "banned@example.com",
    created_at: "2026-05-20T11:00:00Z",
    active: false,
    scope_type: "app",
    scope_id: "client_wordskali",
  },
  // org_ada's shared pool — appears under dsapanicle (org-scoped app).
  {
    user_id: "u_priya",
    username: "priya",
    email: "priya@ada.dev",
    created_at: "2026-05-01T08:15:00Z",
    active: true,
    scope_type: "org",
    scope_id: "org_ada",
  },
];

interface MockSession extends UserSession {
  user_id: string;
  client_id: string;
}

let mockSessions: MockSession[] = [
  s("sess_1", "u_kush", "client_wordskali", "Firefox on macOS", "2026-07-15T09:00:00Z"),
  s("sess_2", "u_kush", "client_wordskali", "Safari on iPhone", "2026-07-17T21:30:00Z"),
  s("sess_3", "u_mina", "client_wordskali", "Chrome on Windows", "2026-07-16T13:10:00Z"),
  s("sess_4", "u_priya", "client_dsapanicle", "Chrome on Android", "2026-07-18T07:45:00Z"),
];

function s(
  session_id: string,
  user_id: string,
  client_id: string,
  user_agent: string,
  created_at: string
): MockSession {
  const expires = new Date(new Date(created_at).getTime() + 7 * 24 * 3600 * 1000);
  return { session_id, user_id, client_id, user_agent, created_at, expires_at: expires.toISOString() };
}

// Write-only demo credentials per app (never returned by updateApp/listApps).
const mockDemoCreds = new Map<string, { email: string; password: string }>();

// fake platform-wide counts for the metrics page (mock has few end users)
const counters = { users: 12_840, active_sessions: 861 };

// ------------------------------------------------------------- helpers

const delay = <T,>(v: T): Promise<T> =>
  new Promise((res) => setTimeout(() => res(v), LATENCY_MS));

const fail = (msg: string): Promise<never> =>
  new Promise((_, rej) => setTimeout(() => rej(new ApiError(msg)), LATENCY_MS));

function randomId(prefix: string): string {
  return `${prefix}_${Math.random().toString(36).slice(2, 10)}`;
}

function randomSecret(): string {
  const chars = "ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz23456789";
  let s = "sk_";
  for (let i = 0; i < 40; i++) s += chars[Math.floor(Math.random() * chars.length)];
  return s;
}

function parseRef(ref: string): { type: string; id: string } | null {
  const i = ref.indexOf(":");
  if (i <= 0 || i === ref.length - 1) return null;
  return { type: ref.slice(0, i), id: ref.slice(i + 1) };
}

function sameKey(a: RelationTuple, b: TupleKey): boolean {
  return (
    a.object_type === b.object_type &&
    a.object_id === b.object_id &&
    a.relation === b.relation &&
    a.subject_type === b.subject_type &&
    a.subject_id === b.subject_id &&
    a.effect === b.effect &&
    (a.condition_expr ?? "") === (b.condition_expr ?? "")
  );
}

// Minimal ABAC condition evaluator: `key == "value"` / `key != "value"`,
// optionally joined with `&&`. Anything unparseable fails closed.
function evalCondition(expr: string, ctx: Record<string, string>): boolean {
  const clauses = expr.split("&&").map((c) => c.trim());
  for (const clause of clauses) {
    const m = clause.match(/^([A-Za-z_][A-Za-z0-9_]*)\s*(==|!=)\s*"([^"]*)"$/);
    if (!m) return false; // parse error → deny (fail closed)
    const [, key, op, value] = m;
    if (!(key in ctx)) return false;
    const eq = ctx[key] === value;
    if (op === "==" ? !eq : eq) return false;
  }
  return true;
}

/** Relations whose grant implies `relation` on this object type (incl. itself). */
function expandRelation(modelJson: string | undefined, objectType: string, relation: string): Set<string> {
  const out = new Set<string>([relation]);
  if (!modelJson) return out;
  let parsed: { types?: Record<string, { relations?: Record<string, string[]> }> };
  try {
    parsed = JSON.parse(modelJson);
  } catch {
    return out;
  }
  const rels = parsed.types?.[objectType]?.relations ?? {};
  let grew = true;
  while (grew) {
    grew = false;
    for (const r of [...out]) {
      for (const implier of rels[r] ?? []) {
        if (!out.has(implier)) {
          out.add(implier);
          grew = true;
        }
      }
    }
  }
  return out;
}

// ------------------------------------------------------------- the client

export class MockApiClient implements ApiClient {
  private currentDev: DevRecord | null = null;

  constructor() {
    // Survive a page reload during a session (mock convenience).
    const email = sessionStorage.getItem("mock_session_email");
    if (email) this.currentDev = developers.find((d) => d.email === email) ?? null;
  }

  // ---- auth ----

  async developerLogin(email: string, password: string): Promise<DeveloperSession> {
    const dev = developers.find((d) => d.email === email.trim().toLowerCase());
    if (!dev || dev.password !== password) return fail("Invalid credentials.");
    this.currentDev = dev;
    sessionStorage.setItem("mock_session_email", dev.email);
    return delay(this.session(dev));
  }

  async registerDeveloper(email: string, password: string): Promise<DeveloperSession> {
    const normalized = email.trim().toLowerCase();
    if (!/^[^@\s]+@[^@\s]+\.[^@\s]+$/.test(normalized)) return fail("Enter a valid email address.");
    if (password.length < 8) return fail("Password must be at least 8 characters.");
    if (developers.some((d) => d.email === normalized)) return fail("Invalid credentials."); // never leak account existence
    const org: Organization = {
      org_id: randomId("org"),
      name: `${normalized.split("@")[0]} — personal org`,
      created_at: now(),
    };
    orgs.push(org);
    const dev: DevRecord = {
      developer_id: randomId("dev"),
      email: normalized,
      password,
      org_id: org.org_id,
      created_at: now(),
    };
    developers.push(dev);
    this.currentDev = dev;
    sessionStorage.setItem("mock_session_email", dev.email);
    return delay(this.session(dev));
  }

  signOut(): void {
    this.currentDev = null;
    sessionStorage.removeItem("mock_session_email");
  }

  restoreSession(): DeveloperSession | null {
    return this.currentDev ? this.session(this.currentDev) : null;
  }

  private session(dev: DevRecord): DeveloperSession {
    const { password: _pw, ...pub } = dev;
    const org = orgs.find((o) => o.org_id === dev.org_id)!;
    return { token: `mock.jwt.${dev.developer_id}`, developer: pub, org };
  }

  private me(): DevRecord {
    if (!this.currentDev) throw new ApiError("Not signed in.");
    return this.currentDev;
  }

  // ---- apps ----

  async listApps(): Promise<App[]> {
    const me = this.me();
    return delay(apps.filter((a) => a.org_id === me.org_id && a.client_id !== PLATFORM_CLIENT_ID).map((a) => ({ ...a })));
  }

  async createApp(name: string, identityScope: IdentityScope) {
    const me = this.me();
    const trimmed = name.trim();
    if (!trimmed) return fail("App name is required.");
    const app: App = {
      client_id: randomId("client"),
      org_id: me.org_id,
      name: trimmed,
      identity_scope: identityScope,
      status: "active",
      created_at: now(),
      redirect_uris: [],
      demo_enabled: false,
      login_identifier: "username_or_email",
      public_signup: false,
    };
    apps.push(app);
    return delay({ app: { ...app }, client_secret: randomSecret() });
  }

  private ownedApp(clientId: string): App {
    const me = this.me();
    const app = apps.find((a) => a.client_id === clientId && a.org_id === me.org_id);
    if (!app) throw new ApiError("App not found.");
    return app;
  }

  async updateApp(
    clientId: string,
    patch: {
      name?: string;
      identity_scope?: IdentityScope;
      redirect_uris?: string[];
      demo_enabled?: boolean;
      demo_email?: string;
      demo_password?: string;
      login_identifier?: LoginIdentifier;
      public_signup?: boolean;
    }
  ): Promise<App> {
    const app = this.ownedApp(clientId);
    if (patch.name !== undefined) {
      if (!patch.name.trim()) return fail("App name is required.");
      app.name = patch.name.trim();
    }
    if (patch.identity_scope !== undefined) app.identity_scope = patch.identity_scope;
    if (patch.redirect_uris !== undefined)
      app.redirect_uris = patch.redirect_uris.map((u) => u.trim()).filter(Boolean);
    if (patch.demo_enabled !== undefined) {
      // Mirror the backend: enabling demo requires stored demo credentials.
      const hasCreds =
        (patch.demo_email ?? "").trim() !== "" && (patch.demo_password ?? "") !== "";
      if (patch.demo_enabled && !hasCreds && !mockDemoCreds.has(clientId))
        return fail("Demo login needs a demo email and password.");
      app.demo_enabled = patch.demo_enabled;
    }
    // demo_email/demo_password are write-only: stored, never returned.
    if ((patch.demo_email ?? "").trim() !== "" && patch.demo_password)
      mockDemoCreds.set(clientId, {
        email: patch.demo_email!.trim(),
        password: patch.demo_password,
      });
    if (patch.login_identifier !== undefined) app.login_identifier = patch.login_identifier;
    if (patch.public_signup !== undefined) app.public_signup = patch.public_signup;
    return delay({ ...app, redirect_uris: [...app.redirect_uris] });
  }

  async rotateAppSecret(clientId: string) {
    this.ownedApp(clientId);
    return delay({ client_secret: randomSecret() });
  }

  async deleteApp(clientId: string): Promise<void> {
    const app = this.ownedApp(clientId);
    apps.splice(apps.indexOf(app), 1);
    models.delete(clientId);
    tuples.delete(clientId);
    return delay(undefined);
  }

  // ---- authz ----

  async getAuthzModel(clientId: string): Promise<AuthzModel | null> {
    this.ownedApp(clientId);
    const m = models.get(clientId);
    return delay(m ? { ...m } : null);
  }

  async writeAuthzModel(clientId: string, modelJson: string): Promise<AuthzModel> {
    this.ownedApp(clientId);
    try {
      const parsed = JSON.parse(modelJson);
      if (typeof parsed !== "object" || parsed === null || Array.isArray(parsed))
        return fail("Model must be a JSON object.");
    } catch {
      return fail("Model is not valid JSON.");
    }
    const prev = models.get(clientId);
    const next: AuthzModel = {
      client_id: clientId,
      model_json: modelJson,
      version: (prev?.version ?? 0) + 1,
      updated_at: now(),
    };
    models.set(clientId, next);
    return delay({ ...next });
  }

  async listTuples(clientId: string, filter?: TupleFilter): Promise<RelationTuple[]> {
    this.ownedApp(clientId);
    let rows = (tuples.get(clientId) ?? []).map((r) => ({ ...r }));
    if (filter?.object) {
      const q = filter.object.toLowerCase();
      rows = rows.filter((r) => `${r.object_type}:${r.object_id}`.toLowerCase().includes(q));
    }
    if (filter?.subject) {
      const q = filter.subject.toLowerCase();
      rows = rows.filter((r) => `${r.subject_type}:${r.subject_id}`.toLowerCase().includes(q));
    }
    return delay(rows);
  }

  async writeTuples(clientId: string, newTuples: RelationTuple[]): Promise<void> {
    this.ownedApp(clientId);
    const rows = tuples.get(clientId) ?? [];
    for (const nt of newTuples) {
      if (!nt.object_type || !nt.object_id || !nt.relation || !nt.subject_id)
        return fail("object, relation, and subject are all required.");
      if (!rows.some((r) => sameKey(r, nt))) rows.push({ ...nt, created_at: now() });
    }
    tuples.set(clientId, rows);
    return delay(undefined);
  }

  async deleteTuples(clientId: string, keys: TupleKey[]): Promise<void> {
    this.ownedApp(clientId);
    const rows = (tuples.get(clientId) ?? []).filter((r) => !keys.some((k) => sameKey(r, k)));
    tuples.set(clientId, rows);
    return delay(undefined);
  }

  async check(
    clientId: string,
    subject: string,
    relation: string,
    object: string,
    context: Record<string, string> = {}
  ): Promise<CheckResult> {
    return delay(this.resolve(clientId, subject, relation.trim(), object, context, 0));
  }

  private resolve(
    clientId: string,
    subject: string,
    relation: string,
    object: string,
    ctx: Record<string, string>,
    depth: number
  ): CheckResult {
    if (depth > 20) return { allowed: false, reason: "recursion depth limit (20) reached" };
    const sub = parseRef(subject);
    const obj = parseRef(object);
    if (!sub || !obj) return { allowed: false, reason: "subject and object must look like type:id" };

    const rows = tuples.get(clientId) ?? [];
    const onObject = rows.filter((r) => r.object_type === obj.type && r.object_id === obj.id);

    const matchesSubject = (r: RelationTuple): CheckResult | null => {
      if (r.condition_expr && !evalCondition(r.condition_expr, ctx))
        return null; // failed/unparseable condition → tuple doesn't match
      if (r.subject_type === sub.type && r.subject_id === sub.id)
        return { allowed: true, reason: `matched tuple ${fmt(r)}` };
      if (r.subject_type === "role") {
        // userset hop: is the subject a member of this role?
        const hop = this.resolve(clientId, subject, "member", `role:${r.subject_id}`, ctx, depth + 1);
        if (hop.allowed) return { allowed: true, reason: `via role:${r.subject_id} → ${fmt(r)}` };
      }
      return null;
    };

    // 1. Deny pass — exact relation only, deny always wins.
    for (const r of onObject) {
      if (r.effect === "deny" && r.relation === relation && matchesSubject(r))
        return { allowed: false, reason: `explicit deny: ${fmt(r)}` };
    }

    // 2. Allow pass — expand relation through the model's implications.
    const expanded = expandRelation(models.get(clientId)?.model_json, obj.type, relation);
    for (const r of onObject) {
      if (r.effect !== "allow" || !expanded.has(r.relation)) continue;
      const hit = matchesSubject(r);
      if (hit) return hit;
    }

    // 3. Default deny.
    return { allowed: false, reason: "no matching tuple (default deny)" };
  }

  async listObjects(clientId: string, subject: string, relation: string, objectType: string): Promise<string[]> {
    this.ownedApp(clientId);
    const rows = tuples.get(clientId) ?? [];
    const ids = new Set(rows.filter((r) => r.object_type === objectType).map((r) => r.object_id));
    const out: string[] = [];
    for (const id of ids) {
      const res = this.resolve(clientId, subject, relation, `${objectType}:${id}`, {}, 0);
      if (res.allowed) out.push(`${objectType}:${id}`);
    }
    return delay(out.sort());
  }

  // ---- app user management ----

  private appEndUsers(app: App): MockEndUser[] {
    return endUsers.filter(
      (u) =>
        (u.scope_type === "app" && u.scope_id === app.client_id) ||
        (app.identity_scope === "org" && u.scope_type === "org" && u.scope_id === app.org_id)
    );
  }

  private scopedUser(app: App, userId: string): MockEndUser {
    const user = this.appEndUsers(app).find((u) => u.user_id === userId);
    if (!user) throw new ApiError("User not found.");
    return user;
  }

  async listAppUsers(clientId: string, query?: string): Promise<AppUser[]> {
    const app = this.ownedApp(clientId);
    let rows = this.appEndUsers(app);
    if (query?.trim()) {
      const q = query.trim().toLowerCase();
      rows = rows.filter(
        (u) => u.username.toLowerCase().includes(q) || u.email.toLowerCase().includes(q)
      );
    }
    return delay(
      rows.map((u) => ({
        user_id: u.user_id,
        username: u.username,
        email: u.email,
        created_at: u.created_at,
        active: u.active,
        session_count: mockSessions.filter(
          (ms) => ms.user_id === u.user_id && ms.client_id === clientId
        ).length,
        scope: u.scope_type,
        lock_username: !!u.lock_username,
        lock_email: !!u.lock_email,
        lock_password: !!u.lock_password,
      }))
    );
  }

  async listUserSessions(clientId: string, userId: string): Promise<UserSession[]> {
    const app = this.ownedApp(clientId);
    this.scopedUser(app, userId);
    return delay(
      mockSessions
        .filter((ms) => ms.user_id === userId && ms.client_id === clientId)
        .map(({ user_id: _u, client_id: _c, ...pub }) => pub)
    );
  }

  async revokeUserSessions(clientId: string, userId: string): Promise<void> {
    const app = this.ownedApp(clientId);
    this.scopedUser(app, userId);
    mockSessions = mockSessions.filter(
      (ms) => !(ms.user_id === userId && ms.client_id === clientId)
    );
    return delay(undefined);
  }

  async setUserActive(clientId: string, userId: string, active: boolean): Promise<string> {
    const app = this.ownedApp(clientId);
    const user = this.scopedUser(app, userId);
    user.active = active;
    if (active) return delay("User reactivated; they can log in again");
    // Deactivation disables the identity, so EVERY session dies — for
    // org-scoped users that spans all of the org's apps.
    mockSessions = mockSessions.filter((ms) => ms.user_id !== userId);
    return delay(
      user.scope_type === "org"
        ? "User deactivated; this is an org-scoped identity, so they are disabled and logged out across ALL of the org's apps"
        : "User deactivated; all of their sessions have been revoked"
    );
  }

  async updateUser(
    clientId: string,
    userId: string,
    patch: {
      username?: string;
      email?: string;
      password?: string;
      lock_username?: boolean;
      lock_email?: boolean;
      lock_password?: boolean;
    }
  ): Promise<AppUser> {
    const app = this.ownedApp(clientId);
    const user = this.scopedUser(app, userId);
    if (patch.username !== undefined) {
      if (!patch.username.trim()) return fail("Username cannot be empty.");
      user.username = patch.username.trim();
    }
    if (patch.email !== undefined) {
      if (!/^[^@\s]+@[^@\s]+\.[^@\s]+$/.test(patch.email.trim()))
        return fail("Enter a valid email address.");
      user.email = patch.email.trim();
    }
    // patch.password is write-only in the mock (no stored credential store).
    if (patch.lock_username !== undefined) user.lock_username = patch.lock_username;
    if (patch.lock_email !== undefined) user.lock_email = patch.lock_email;
    if (patch.lock_password !== undefined) user.lock_password = patch.lock_password;
    return delay({
      user_id: user.user_id,
      username: user.username,
      email: user.email,
      created_at: user.created_at,
      active: user.active,
      session_count: mockSessions.filter(
        (ms) => ms.user_id === user.user_id && ms.client_id === clientId
      ).length,
      scope: user.scope_type,
      lock_username: !!user.lock_username,
      lock_email: !!user.lock_email,
      lock_password: !!user.lock_password,
    });
  }

  // ---- superadmin ----

  private requireSuperadmin(): void {
    const me = this.me();
    const res = this.resolve(PLATFORM_CLIENT_ID, `user:${me.developer_id}`, "admin", "app:platform", {}, 0);
    if (!res.allowed) throw new ApiError("Permission denied.");
  }

  async listAllOrgs(): Promise<Organization[]> {
    this.requireSuperadmin();
    return delay(orgs.map((o) => ({ ...o })));
  }

  async listAllApps(): Promise<App[]> {
    this.requireSuperadmin();
    return delay(apps.map((a) => ({ ...a })));
  }

  async suspendClient(clientId: string): Promise<App> {
    this.requireSuperadmin();
    const app = apps.find((a) => a.client_id === clientId);
    if (!app) return fail("App not found.");
    if (app.client_id === PLATFORM_CLIENT_ID) return fail("The platform client cannot be suspended.");
    app.status = "suspended";
    return delay({ ...app });
  }

  async restoreClient(clientId: string): Promise<App> {
    this.requireSuperadmin();
    const app = apps.find((a) => a.client_id === clientId);
    if (!app) return fail("App not found.");
    app.status = "active";
    return delay({ ...app });
  }

  async listDevelopers(): Promise<Developer[]> {
    this.requireSuperadmin();
    return delay(developers.map(({ password: _pw, ...d }) => d));
  }

  async getPlatformMetrics(): Promise<PlatformMetrics> {
    this.requireSuperadmin();
    let tupleCount = 0;
    for (const rows of tuples.values()) tupleCount += rows.length;
    return delay({
      orgs: orgs.length,
      apps: apps.length,
      developers: developers.length,
      users: counters.users,
      active_sessions: counters.active_sessions,
      tuples: tupleCount,
    });
  }
}

function fmt(r: RelationTuple): string {
  return `${r.subject_type}:${r.subject_id} —${r.relation}→ ${r.object_type}:${r.object_id}`;
}
