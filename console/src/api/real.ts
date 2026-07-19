// Real backend client. Transport per the backend contract:
//   HTTP POST /auth.v1.PlatformService/{MethodName}
//   JSON bodies in protojson style (camelCase fields)
// In dev, vite proxies /auth.v1 → http://localhost:8081 (see vite.config.ts),
// so no CORS setup is needed.
//
// Token refresh: on an expired/invalid-token response the client attempts ONE
// DeveloperRefreshToken call with the stored refresh token, persists the
// rotated pair, and retries the original request once. Parallel requests share
// a single in-flight refresh promise. If the refresh fails, the session is
// cleared and a "session expired, sign in again" ApiError is surfaced.
// The client also refreshes proactively ~60s before the access token's exp
// (JWTs expire in 30 min), via one timer reset on login/refresh and cleared
// on signOut.

import {
  ApiClient,
  ApiError,
  App,
  AppUser,
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
  UserScope,
  UserSession,
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
  redirectUris?: string[];
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

interface WireAppUser {
  userId: string;
  username?: string;
  email?: string;
  createdAt?: string;
  active?: boolean;
  sessionCount?: number | string;
  scope?: UserScope;
}

interface WireSessionInfo {
  sessionId: string;
  userAgent?: string;
  createdAt?: string;
  expiresAt?: string;
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
    redirect_uris: w.redirectUris ?? [],
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

/** Internal marker: the backend rejected the current access token. */
class AuthFailure extends ApiError {}

export class RealApiClient implements ApiClient {
  private stored: StoredSession | null = null;
  /** Single in-flight refresh, shared by parallel requests. */
  private refreshing: Promise<boolean> | null = null;
  private refreshTimer: ReturnType<typeof setTimeout> | undefined;

  constructor() {
    try {
      const raw = sessionStorage.getItem(SESSION_KEY);
      if (raw) this.stored = JSON.parse(raw) as StoredSession;
    } catch {
      this.stored = null;
    }
    if (this.stored) this.scheduleRefresh();
  }

  // ---- transport ----

  private async post<T>(method: string, body: Record<string, unknown>): Promise<T> {
    try {
      return await this.rawPost<T>(method, body);
    } catch (e) {
      if (!(e instanceof AuthFailure)) throw e;
      // Expired/invalid token: try ONE refresh, then retry the request once.
      if (!(await this.tryRefresh())) this.expire();
      const retryBody =
        "accessToken" in body ? { ...body, accessToken: this.token() } : body;
      try {
        return await this.rawPost<T>(method, retryBody);
      } catch (e2) {
        if (e2 instanceof AuthFailure) this.expire();
        throw e2;
      }
    }
  }

  private async rawPost<T>(method: string, body: Record<string, unknown>): Promise<T> {
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

    let data: unknown = null;
    try {
      data = await res.json();
    } catch {
      // non-JSON body; fall through to status handling
    }
    const rec = (data ?? {}) as Record<string, unknown>;
    const message = typeof rec.message === "string" ? rec.message : "";

    if (res.status === 401 && this.stored)
      throw new AuthFailure(message || "Unauthorized.", 401);
    if (!res.ok) {
      if (this.looksLikeAuthFailure(message)) throw new AuthFailure(message, res.status);
      throw new ApiError(message || `Request failed (HTTP ${res.status}).`, res.status);
    }
    if (typeof rec.success === "boolean" && !rec.success) {
      if (this.looksLikeAuthFailure(message)) throw new AuthFailure(message);
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
    this.clearSession();
    throw new ApiError("Your session has expired. Please sign in again.", 401);
  }

  private clearSession(): void {
    this.stored = null;
    sessionStorage.removeItem(SESSION_KEY);
    clearTimeout(this.refreshTimer);
    this.refreshTimer = undefined;
  }

  // ---- token refresh ----

  /** Refresh the token pair; parallel callers await the same attempt. */
  private tryRefresh(): Promise<boolean> {
    if (!this.refreshing) {
      this.refreshing = this.doRefresh().finally(() => {
        this.refreshing = null;
      });
    }
    return this.refreshing;
  }

  private async doRefresh(): Promise<boolean> {
    const refreshToken = this.stored?.refreshToken;
    if (!refreshToken) return false;
    try {
      const r = await this.rawPost<{ accessToken?: string; refreshToken?: string }>(
        "DeveloperRefreshToken",
        { refreshToken }
      );
      if (!r.accessToken || !r.refreshToken || !this.stored) return false;
      // Rotation: the returned refresh token replaces the old one.
      this.stored = {
        ...this.stored,
        accessToken: r.accessToken,
        refreshToken: r.refreshToken,
      };
      sessionStorage.setItem(SESSION_KEY, JSON.stringify(this.stored));
      this.scheduleRefresh();
      return true;
    } catch {
      return false;
    }
  }

  /** exp claim (seconds) from a JWT's base64url payload, or null. */
  private decodeExp(token: string): number | null {
    try {
      const payload = token.split(".")[1];
      const json = JSON.parse(atob(payload.replace(/-/g, "+").replace(/_/g, "/"))) as {
        exp?: unknown;
      };
      return typeof json.exp === "number" ? json.exp : null;
    } catch {
      return null;
    }
  }

  /** One timer that refreshes ~60s before the access token expires. */
  private scheduleRefresh(): void {
    clearTimeout(this.refreshTimer);
    this.refreshTimer = undefined;
    if (!this.stored?.refreshToken) return;
    const exp = this.decodeExp(this.stored.accessToken);
    if (exp === null) return;
    const delayMs = Math.max(exp * 1000 - Date.now() - 60_000, 5_000);
    this.refreshTimer = setTimeout(() => void this.tryRefresh(), delayMs);
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
    this.scheduleRefresh();
    return { token: r.accessToken, developer, org };
  }

  async registerDeveloper(email: string, password: string): Promise<DeveloperSession> {
    // RegisterDeveloper returns ids only (no tokens); log in right after.
    await this.post("RegisterDeveloper", { email: email.trim().toLowerCase(), password });
    return this.developerLogin(email, password);
  }

  signOut(): void {
    this.clearSession();
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
    patch: { name?: string; identity_scope?: IdentityScope; redirect_uris?: string[] }
  ): Promise<App> {
    const r = await this.post<{ app: WireApp }>("UpdateApp", {
      accessToken: this.token(),
      clientId,
      ...(patch.name !== undefined ? { name: patch.name } : {}),
      ...(patch.identity_scope !== undefined ? { identityScope: patch.identity_scope } : {}),
      // Repeated fields cannot express "unset": the flag tells the backend
      // to replace the whitelist (empty list disables hosted login).
      ...(patch.redirect_uris !== undefined
        ? { redirectUris: patch.redirect_uris, setRedirectUris: true }
        : {}),
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

  // ---- app user management ----

  async listAppUsers(clientId: string, query?: string): Promise<AppUser[]> {
    const r = await this.post<{ users?: WireAppUser[] }>("ListAppUsers", {
      accessToken: this.token(),
      clientId,
      ...(query ? { query } : {}),
    });
    return (r.users ?? []).map((w) => ({
      user_id: w.userId,
      username: w.username ?? "",
      email: w.email ?? "",
      created_at: w.createdAt ?? "",
      active: !!w.active,
      session_count: num(w.sessionCount),
      scope: w.scope === "org" ? "org" : "app",
    }));
  }

  async listUserSessions(clientId: string, userId: string): Promise<UserSession[]> {
    const r = await this.post<{ sessions?: WireSessionInfo[] }>("ListUserSessionsAdmin", {
      accessToken: this.token(),
      clientId,
      userId,
    });
    return (r.sessions ?? []).map((w) => ({
      session_id: w.sessionId,
      user_agent: w.userAgent ?? "",
      created_at: w.createdAt ?? "",
      expires_at: w.expiresAt ?? "",
    }));
  }

  async revokeUserSessions(clientId: string, userId: string): Promise<void> {
    await this.post("RevokeUserSessionsAdmin", {
      accessToken: this.token(),
      clientId,
      userId,
    });
  }

  async setUserActive(clientId: string, userId: string, active: boolean): Promise<string> {
    const r = await this.post<{ message?: string }>("SetUserActive", {
      accessToken: this.token(),
      clientId,
      userId,
      active,
    });
    return r.message ?? (active ? "User reactivated." : "User deactivated.");
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
