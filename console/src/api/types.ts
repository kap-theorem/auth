// Typed API surface matching the design spec's RPCs (docs/superpowers/specs/
// 2026-07-18-iam-platform-design.md, §6 authz, §7 tenant mgmt, §8 console).
// The real grpc-gateway JSON client will implement this same interface.

export type IdentityScope = "app" | "org";
export type ClientStatus = "active" | "suspended";
export type Effect = "allow" | "deny";
export type SubjectType = "user" | "role";

export interface Organization {
  org_id: string;
  name: string;
  created_at: string;
}

export interface Developer {
  developer_id: string;
  email: string;
  org_id: string;
  created_at: string;
}

export interface App {
  client_id: string;
  org_id: string;
  name: string;
  identity_scope: IdentityScope;
  status: ClientStatus;
  created_at: string;
}

export interface AuthzModel {
  client_id: string;
  model_json: string;
  version: number;
  updated_at: string;
}

export interface RelationTuple {
  object_type: string;
  object_id: string;
  relation: string;
  subject_type: SubjectType;
  subject_id: string;
  effect: Effect;
  condition_expr?: string;
  created_at?: string;
}

/** Key identifying a tuple for deletion (matches the table's primary key). */
export type TupleKey = Omit<RelationTuple, "created_at">;

export interface DeveloperSession {
  token: string;
  developer: Developer;
  org: Organization;
}

export interface CheckResult {
  allowed: boolean;
  /** Debug hint from the resolver (mock-only convenience; never a security signal). */
  reason?: string;
}

/** Simple platform-wide counts (matches the backend contract's GetPlatformMetrics). */
export interface PlatformMetrics {
  orgs: number;
  apps: number;
  developers: number;
  users: number;
  active_sessions: number;
  tuples: number;
}

export interface TupleFilter {
  object?: string; // "type:id", "type:" or bare text matched against object
  subject?: string; // same, matched against subject
}

export class ApiError extends Error {
  /** HTTP status when the failure came from the transport (real client only). */
  readonly status?: number;
  constructor(message: string, status?: number) {
    super(message);
    this.name = "ApiError";
    this.status = status;
  }
}

export interface ApiClient {
  // ---- Tenant management (§7) ----
  registerDeveloper(email: string, password: string): Promise<DeveloperSession>;
  developerLogin(email: string, password: string): Promise<DeveloperSession>;
  /** Clear the local session (sessionStorage + memory). */
  signOut(): void;
  /** Synchronously restore a session persisted in sessionStorage, if any. */
  restoreSession(): DeveloperSession | null;

  createApp(
    name: string,
    identityScope: IdentityScope
  ): Promise<{ app: App; client_secret: string }>;
  listApps(): Promise<App[]>;
  updateApp(
    clientId: string,
    patch: { name?: string; identity_scope?: IdentityScope }
  ): Promise<App>;
  rotateAppSecret(clientId: string): Promise<{ client_secret: string }>;
  deleteApp(clientId: string): Promise<void>;

  // ---- Authorization (§6) ----
  writeAuthzModel(clientId: string, modelJson: string): Promise<AuthzModel>;
  getAuthzModel(clientId: string): Promise<AuthzModel | null>;
  writeTuples(clientId: string, tuples: RelationTuple[]): Promise<void>;
  deleteTuples(clientId: string, tuples: TupleKey[]): Promise<void>;
  /** Console read helper; the gateway will expose an equivalent ReadTuples. */
  listTuples(clientId: string, filter?: TupleFilter): Promise<RelationTuple[]>;
  check(
    clientId: string,
    subject: string, // "user:kush" | "role:moderator"
    relation: string,
    object: string, // "problem:two-sum"
    context?: Record<string, string>
  ): Promise<CheckResult>;
  listObjects(
    clientId: string,
    subject: string,
    relation: string,
    objectType: string
  ): Promise<string[]>;

  // ---- Superadmin (§7; backend re-checks Check(platform, me, admin, app:platform)) ----
  listAllOrgs(): Promise<Organization[]>;
  listAllApps(): Promise<App[]>;
  suspendClient(clientId: string): Promise<App>;
  restoreClient(clientId: string): Promise<App>;
  listDevelopers(): Promise<Developer[]>;
  getPlatformMetrics(): Promise<PlatformMetrics>;
}

/** The built-in platform client (spec §8): superadmin = tuple on this client. */
export const PLATFORM_CLIENT_ID = "client_platform";
