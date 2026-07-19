// Real backend client. Transport per the backend contract:
//   HTTP POST /auth.v1.PlatformService/{MethodName}
//   JSON bodies in protojson style (camelCase fields)
// In dev, vite proxies /auth.v1 → http://localhost:8081 (see vite.config.ts),
// so no CORS setup is needed.
//
// Token refresh is intentionally NOT implemented: the only refresh RPC in the
// proto (AuthService.RefreshToken) requires a client_secret, which the console
// never holds, and the platform contract exposes no developer refresh method.
// On a 401 / invalid-token style response we clear the session and surface a
// "session expired, sign in again" ApiError instead.

import {
  ApiClient,
  ApiError,
  App,
  AuthzModel,
  CheckResult,
  Developer,
  DeveloperSession,
  Effect,
  IdentityScope,
  Organization,
  PlatformMetrics,
  RelationTuple,
  SubjectType,
  TupleFilter,
  TupleKey,
} from "./types";

const BASE = "/auth.v1.PlatformService";
const SESSION_KEY = "real_session";

// ------------------------------------------------------------- wire shapes

interface WireApp {
  clientId: string;
  name: string;
  orgId: string;
  identityScope: IdentityScope;
  suspended?: boolean;
  createdAt?: string;
}

interface WireTuple {
  objectType: string;
  objectId: string;
  relation: string;
  subjectType: SubjectType;
  subjectId: string;
  effect: Effect;
  conditionExpr?: string;
}

interface WireOrg {
  orgId: string;
  name?: string;
  orgName?: string;
  createdAt?: string;
}

interface WireDeveloper {
  developerId: string;
  email: string;
  orgId: string;
  orgName?: string;
  createdAt?: string;
}

// ------------------------------------------------------------ thin mappers

function mapApp(w: WireApp): App {
  return {
    client_id: w.clientId,
    org_id: w.orgId,
    name: w.name,
    identity_scope: w.identityScope,
    status: w.suspended ? "suspended" : "active",
    created_at: w.createdAt ?? "",
  };
}

function mapDeveloper(w: WireDeveloper): Developer {
  return {
    developer_id: w.developerId,
    email: w.email,
    org_id: w.orgId,
    created_at: w.createdAt ?? "",
  };
}

function mapOrg(w: WireOrg): Organization {
  return {
    org_id: w.orgId,
    name: w.name ?? w.orgName ?? "",
    created_at: w.createdAt ?? "",
  };
}

function toWireTuple(t: RelationTuple | TupleKey): WireTuple {
  return {
    objectType: t.object_type,
    objectId: t.object_id,
    relation: t.relation,
    subjectType: t.subject_type,
    subjectId: t.subject_id,
    effect: t.effect,
    ...(t.condition_expr ? { conditionExpr: t.condition_expr } : {}),
  };
}

function fromWireTuple(w: WireTuple): RelationTuple {
  return {
    object_type: w.objectType,
    object_id: w.objectId,
    relation: w.relation,
    subject_type: w.subjectType,
    subject_id: w.subjectId,
    effect: w.effect,
    condition_expr: w.conditionExpr || undefined,
  };
}

function parseRef(ref: string): { type: string; id: string } {
  const i = ref.indexOf(":");
  if (i <= 0 || i === ref.length - 1)
    throw new ApiError('Object must look like "type:id".');
  return { type: ref.slice(0, i), id: ref.slice(i + 1) };
}

// protojson emits int64 as strings ("2"), so accept both
const num = (v: unknown): number =>
  typeof v === "number" ? v : typeof v === "string" ? Number(v) || 0 : 0;

// -------------------------------------------------------------- the client

interface StoredSession {
  accessToken: string;
  refreshToken?: string;
  developer: Developer;
  org: Organization;
}

export class RealApiClient implements ApiClient {
  private stored: StoredSession | null = null;

  constructor() {
    try {
      const raw = sessionStorage.getItem(SESSION_KEY);
      if (raw) this.stored = JSON.parse(raw) as StoredSession;
    } catch {
      this.stored = null;
    }
  }

  // ---- transport ----

  private async post<T>(method: string, body: Record<string, unknown>): Promise<T> {
    let res: Response;
    try {
      res = await fetch(`${BASE}/${method}`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(body),
      });
    } catch {
      throw new ApiError("Cannot reach the backend (is it running on :8081?).");
    }
    if (res.status === 401 && this.stored) this.expire();

    let data: unknown = null;
    try {
      data = await res.json();
    } catch {
      // non-JSON body; fall through to status handling
    }
    const rec = (data ?? {}) as Record<string, unknown>;
    const message = typeof rec.message === "string" ? rec.message : "";

    if (!res.ok) {
      if (this.looksLikeAuthFailure(message)) this.expire();
      throw new ApiError(message || `Request failed (HTTP ${res.status}).`, res.status);
    }
    if (typeof rec.success === "boolean" && !rec.success) {
      if (this.looksLikeAuthFailure(message)) this.expire();
      throw new ApiError(message || "Request failed.");
    }
    return rec as T;
  }

  private looksLikeAuthFailure(message: string): boolean {
    // Only meaningful when a session exists (never confuse a failed login
    // attempt's "invalid credentials" with an expired session).
    return (
      this.stored !== null &&
      /invalid token|expired|unauthenticated|unauthorized|session/i.test(message)
    );
  }

  private expire(): never {
    this.stored = null;
    sessionStorage.removeItem(SESSION_KEY);
    throw new ApiError("Your session has expired. Please sign in again.", 401);
  }

  private token(): string {
    if (!this.stored) throw new ApiError("Not signed in.");
    return this.stored.accessToken;
  }

  // ---- auth / session ----

  async developerLogin(email: string, password: string): Promise<DeveloperSession> {
    const r = await this.post<{
      accessToken: string;
      refreshToken?: string;
      developer: WireDeveloper;
    }>("DeveloperLogin", { email: email.trim().toLowerCase(), password });
    if (!r.accessToken || !r.developer)
      throw new ApiError("Malformed login response from the backend.");
    const developer = mapDeveloper(r.developer);
    const org: Organization = {
      org_id: r.developer.orgId,
      name: r.developer.orgName ?? "",
      created_at: "",
    };
    this.stored = { accessToken: r.accessToken, refreshToken: r.refreshToken, developer, org };
    sessionStorage.setItem(SESSION_KEY, JSON.stringify(this.stored));
    return { token: r.accessToken, developer, org };
  }

  async registerDeveloper(email: string, password: string): Promise<DeveloperSession> {
    // RegisterDeveloper returns ids only (no tokens); log in right after.
    await this.post("RegisterDeveloper", { email: email.trim().toLowerCase(), password });
    return this.developerLogin(email, password);
  }

  signOut(): void {
    this.stored = null;
    sessionStorage.removeItem(SESSION_KEY);
  }

  restoreSession(): DeveloperSession | null {
    return this.stored
      ? { token: this.stored.accessToken, developer: this.stored.developer, org: this.stored.org }
      : null;
  }

  // ---- apps ----

  async listApps(): Promise<App[]> {
    const r = await this.post<{ apps?: WireApp[] }>("ListApps", { accessToken: this.token() });
    return (r.apps ?? []).map(mapApp);
  }

  async createApp(name: string, identityScope: IdentityScope) {
    const r = await this.post<{ app: WireApp; clientSecret: string }>("CreateApp", {
      accessToken: this.token(),
      name,
      identityScope,
    });
    return { app: mapApp(r.app), client_secret: r.clientSecret };
  }

  async updateApp(
    clientId: string,
    patch: { name?: string; identity_scope?: IdentityScope }
  ): Promise<App> {
    const r = await this.post<{ app: WireApp }>("UpdateApp", {
      accessToken: this.token(),
      clientId,
      ...(patch.name !== undefined ? { name: patch.name } : {}),
      ...(patch.identity_scope !== undefined ? { identityScope: patch.identity_scope } : {}),
    });
    return mapApp(r.app);
  }

  async rotateAppSecret(clientId: string) {
    const r = await this.post<{ clientSecret: string }>("RotateAppSecret", {
      accessToken: this.token(),
      clientId,
    });
    return { client_secret: r.clientSecret };
  }

  async deleteApp(clientId: string): Promise<void> {
    await this.post("DeleteApp", { accessToken: this.token(), clientId });
  }

  // ---- authz ----

  async writeAuthzModel(clientId: string, modelJson: string): Promise<AuthzModel> {
    const r = await this.post<{ version?: number }>("WriteAuthzModel", {
      accessToken: this.token(),
      clientId,
      modelJson,
    });
    return {
      client_id: clientId,
      model_json: modelJson,
      version: num(r.version),
      updated_at: new Date().toISOString(),
    };
  }

  async getAuthzModel(clientId: string): Promise<AuthzModel | null> {
    let r: { modelJson?: string; version?: number };
    try {
      r = await this.post("GetAuthzModel", { accessToken: this.token(), clientId });
    } catch (e) {
      // "no model yet" is a normal state for a fresh app, not an error
      if (e instanceof ApiError && /not.?found|no (authz )?model/i.test(e.message)) return null;
      throw e;
    }
    if (!r.modelJson) return null;
    return {
      client_id: clientId,
      model_json: r.modelJson,
      version: num(r.version),
      updated_at: "",
    };
  }

  async writeTuples(clientId: string, tuples: RelationTuple[]): Promise<void> {
    await this.post("WriteTuples", {
      accessToken: this.token(),
      clientId,
      tuples: tuples.map(toWireTuple),
    });
  }

  async deleteTuples(clientId: string, tuples: TupleKey[]): Promise<void> {
    await this.post("DeleteTuples", {
      accessToken: this.token(),
      clientId,
      tuples: tuples.map(toWireTuple),
    });
  }

  async listTuples(clientId: string, filter?: TupleFilter): Promise<RelationTuple[]> {
    // The wire filter is exact (objectType/objectId/...); the console's filter
    // is substring text. Fetch all rows for the client and filter here, with
    // the same semantics as the mock.
    const r = await this.post<{ tuples?: WireTuple[] }>("ListTuples", {
      accessToken: this.token(),
      clientId,
    });
    let rows = (r.tuples ?? []).map(fromWireTuple);
    if (filter?.object) {
      const q = filter.object.toLowerCase();
      rows = rows.filter((t) => `${t.object_type}:${t.object_id}`.toLowerCase().includes(q));
    }
    if (filter?.subject) {
      const q = filter.subject.toLowerCase();
      rows = rows.filter((t) => `${t.subject_type}:${t.subject_id}`.toLowerCase().includes(q));
    }
    return rows;
  }

  async check(
    clientId: string,
    subject: string,
    relation: string,
    object: string,
    context: Record<string, string> = {}
  ): Promise<CheckResult> {
    const obj = parseRef(object);
    // NOTE: the contract's Check omits clientId, but every sibling RPC scopes
    // by it and the console checks against per-app tuples; we send it so a
    // backend that needs it gets it (extra JSON fields are ignored otherwise).
    const r = await this.post<{ allowed?: boolean; reason?: string }>("Check", {
      accessToken: this.token(),
      clientId,
      subject,
      relation,
      objectType: obj.type,
      objectId: obj.id,
      context,
    });
    return { allowed: !!r.allowed, reason: r.reason };
  }

  async listObjects(): Promise<string[]> {
    // Not part of the backend contract yet; the console does not call it.
    throw new ApiError("ListObjects is not available on the real backend yet.");
  }

  // ---- superadmin ----

  async listAllOrgs(): Promise<Organization[]> {
    const r = await this.post<{ orgs?: WireOrg[] }>("ListAllOrgs", {
      accessToken: this.token(),
    });
    return (r.orgs ?? []).map(mapOrg);
  }

  async listAllApps(): Promise<App[]> {
    const r = await this.post<{ apps?: WireApp[] }>("ListAllApps", {
      accessToken: this.token(),
    });
    return (r.apps ?? []).map(mapApp);
  }

  async suspendClient(clientId: string): Promise<App> {
    return this.adminToggle("SuspendClient", clientId);
  }

  async restoreClient(clientId: string): Promise<App> {
    return this.adminToggle("RestoreClient", clientId);
  }

  private async adminToggle(method: string, clientId: string): Promise<App> {
    const r = await this.post<{ app?: WireApp }>(method, {
      accessToken: this.token(),
      clientId,
    });
    if (r.app) return mapApp(r.app);
    // Contract only guarantees {success, message}; refetch the app if absent.
    const app = (await this.listAllApps()).find((a) => a.client_id === clientId);
    if (!app) throw new ApiError("App not found.");
    return app;
  }

  async listDevelopers(): Promise<Developer[]> {
    const r = await this.post<{ developers?: WireDeveloper[] }>("ListDevelopers", {
      accessToken: this.token(),
    });
    return (r.developers ?? []).map(mapDeveloper);
  }

  async getPlatformMetrics(): Promise<PlatformMetrics> {
    const r = await this.post<Record<string, unknown>>("GetPlatformMetrics", {
      accessToken: this.token(),
    });
    // Tolerate both flat counts and a nested {metrics: {...}} envelope.
    const m = (r.metrics ?? r) as Record<string, unknown>;
    return {
      orgs: num(m.orgs),
      apps: num(m.apps),
      developers: num(m.developers),
      users: num(m.users),
      active_sessions: num(m.activeSessions),
      tuples: num(m.tuples),
    };
  }
}
