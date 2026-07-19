// Typed API surface matching the design spec's RPCs (docs/superpowers/specs/
// 2026-07-18-iam-platform-design.md, §6 authz, §7 tenant mgmt, §8 console).
// The real grpc-gateway JSON client will implement this same interface.

export type IdentityScope = "app" | "org";
export type ClientStatus = "active" | "suspended";
export type Effect = "allow" | "deny";
export type SubjectType = "user" | "role";
/** Which identifier a hosted-login user types (drives the input's label). */
export type LoginIdentifier = "username_or_email" | "email_only" | "username_only";

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
  /** Exact redirect URIs whitelisted for hosted login. Empty = disabled. */
  redirect_uris: string[];
  /** One-click demo login on the hosted login page. */
  demo_enabled: boolean;
  /** Which identifier hosted-login users authenticate with. */
  login_identifier: LoginIdentifier;
  /** Whether the hosted login page offers public self-service signup. */
  public_signup: boolean;
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
  /** Debug hint from the resolver (shown under the Check verdict; never a security signal). */
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

/** Where an app user's identity lives (spec "App user management"). */
export type UserScope = "app" | "org";

/** One end user of an app, as seen by its developer. */
export interface AppUser {
  user_id: string;
  username: string;
  email: string;
  created_at: string;
  active: boolean;
  /** Active sessions under THIS app. */
  session_count: number;
  /** "org" users are shared across the org's apps — deactivation is org-wide. */
  scope: UserScope;
  /** Self-edit locks: when true, the user cannot change this field themselves. */
  lock_username: boolean;
  lock_email: boolean;
  lock_password: boolean;
}

/** One of an end user's sessions, listed by an app admin. */
export interface UserSession {
  session_id: string;
  user_agent: string;
  created_at: string;
  expires_at: string;
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
    patch: {
      name?: string;
      identity_scope?: IdentityScope;
      redirect_uris?: string[];
      demo_enabled?: boolean;
      /** Write-only demo credentials (never returned). Required to enable demo. */
      demo_email?: string;
      demo_password?: string;
      login_identifier?: LoginIdentifier;
      public_signup?: boolean;
    }
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

  // ---- App user management (spec "App user management") ----
  /** Lists an app's end users; query is a substring match on username OR email. */
  listAppUsers(clientId: string, query?: string): Promise<AppUser[]>;
  listUserSessions(clientId: string, userId: string): Promise<UserSession[]>;
  /** Revokes ALL of one user's sessions under this app. */
  revokeUserSessions(clientId: string, userId: string): Promise<void>;
  /**
   * Activates/deactivates a user. Deactivation revokes every session the
   * user holds (across ALL of the org's apps for org-scoped users — the
   * identity is disabled, not one app's access) and blocks all logins.
   * Resolves to the backend's human-readable outcome message.
   */
  setUserActive(clientId: string, userId: string, active: boolean): Promise<string>;
  /**
   * Admin self-service edit of an app user: change username/email/password and
   * toggle per-field self-edit locks. Omit a field to leave it unchanged.
   * Resolves to the updated user.
   */
  updateUser(
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
  ): Promise<AppUser>;

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
