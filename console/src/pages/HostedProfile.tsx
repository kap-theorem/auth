// Public hosted account page (spec "Hosted account page", added 2026-07-18).
//
// Served at /client/:clientId/user/profile with NO console session and
// minimal chrome, designed to run inside a consumer app's iframe (same look
// as HostedLogin). It always talks to the real backend directly, regardless
// of VITE_API_MODE — the mock API has no end users.
//
// Token acquisition, in priority order:
//   1. #access_token=<jwt> URL fragment (top-level flow; fragments never
//      reach server logs).
//   2. postMessage handshake: on mount, if embedded, the page posts
//      {type: "PROFILE_READY"} to the parent and waits for
//      {type: "AUTH_TOKEN", access_token}. No token after ~2s renders a
//      friendly "not signed in" state.
//
// After a successful HostedLogoutAll the page posts {type: "LOGGED_OUT"} to
// the embedding app so it can clear its cookie and redirect.

import { FormEvent, useCallback, useEffect, useRef, useState } from "react";
import { useParams } from "react-router-dom";

const BASE = "/auth.v1.PlatformService";

interface UserProfile {
  userId?: string;
  username?: string;
  email?: string;
}

interface SessionInfo {
  sessionId: string;
  userAgent?: string;
  createdAt?: string;
  expiresAt?: string;
  current?: boolean;
}

interface ProfileResponse {
  success: boolean;
  message?: string;
  user?: UserProfile;
  sessions?: SessionInfo[];
}

interface SimpleResponse {
  success: boolean;
  message?: string;
}

// Direct fetch: this page must bypass the mock/real API switch.
async function post<T>(method: string, body: Record<string, unknown>): Promise<T> {
  let res: Response;
  try {
    res = await fetch(`${BASE}/${method}`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    });
  } catch {
    throw new Error("Cannot reach the account service. Please try again.");
  }
  let data: unknown = null;
  try {
    data = await res.json();
  } catch {
    // fall through
  }
  const rec = (data ?? {}) as Record<string, unknown>;
  if (!res.ok) {
    const msg = typeof rec.message === "string" ? rec.message : "";
    throw new Error(msg || `Request failed (HTTP ${res.status}).`);
  }
  return rec as T;
}

function formatDate(iso?: string): string {
  if (!iso) return "";
  const d = new Date(iso);
  return isNaN(d.getTime()) ? "" : d.toLocaleString();
}

// Truncate long user-agent strings for the session table.
function shortAgent(ua?: string): string {
  if (!ua) return "Unknown device";
  return ua.length > 72 ? ua.slice(0, 72) + "…" : ua;
}

export default function HostedProfile() {
  const { clientId = "" } = useParams();

  // null = still acquiring; "" = acquisition finished without a token.
  const [token, setToken] = useState<string | null>(null);
  // Origin of the parent frame that sent AUTH_TOKEN; used as the target
  // origin for every message we post back after the handshake.
  const parentOrigin = useRef<string | null>(null);

  const [profile, setProfile] = useState<ProfileResponse | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const [loggedOut, setLoggedOut] = useState(false);

  // Profile self-edit state.
  const [editing, setEditing] = useState(false);
  const [editUsername, setEditUsername] = useState("");
  const [editEmail, setEditEmail] = useState("");
  const [saveBusy, setSaveBusy] = useState(false);

  // Change-password form state.
  const [currentPw, setCurrentPw] = useState("");
  const [newPw, setNewPw] = useState("");
  const [confirmPw, setConfirmPw] = useState("");
  const [pwBusy, setPwBusy] = useState(false);

  // ---- Token acquisition -------------------------------------------------
  useEffect(() => {
    // (a) URL fragment: #access_token=<jwt>
    const frag = new URLSearchParams(window.location.hash.replace(/^#/, ""));
    const fragToken = frag.get("access_token");
    if (fragToken) {
      // Scrub the token from the address bar (it stays out of history).
      window.history.replaceState(null, "", window.location.pathname + window.location.search);
      setToken(fragToken);
      return;
    }

    // (b) postMessage handshake with the embedding app.
    if (window.parent === window) {
      setToken(""); // top-level with no fragment: nothing to wait for
      return;
    }

    const onMessage = (event: MessageEvent) => {
      // Accept AUTH_TOKEN only from a plausible web origin (an iframe parent
      // is always http(s); this rejects e.g. "null" from sandboxed frames).
      if (!/^https?:\/\//.test(event.origin)) return;
      const data = event.data as { type?: string; access_token?: string } | null;
      if (data && data.type === "AUTH_TOKEN" && typeof data.access_token === "string" && data.access_token) {
        // Remember the sender's origin so every reply (e.g. LOGGED_OUT) is
        // scoped to that exact frame, never "*".
        parentOrigin.current = event.origin;
        setToken(data.access_token);
      }
    };
    window.addEventListener("message", onMessage);

    // Announce readiness. targetOrigin "*" is acceptable ONLY for this ping:
    // it carries no secret (a fixed type string), and at this point we
    // cannot know the embedding app's origin yet — the handshake exists to
    // learn it. Every later message uses the verified origin above.
    window.parent.postMessage({ type: "PROFILE_READY" }, "*");

    // ~2s with no AUTH_TOKEN → friendly "not signed in" state.
    const timer = window.setTimeout(() => setToken((t) => (t === null ? "" : t)), 2000);
    return () => {
      window.removeEventListener("message", onMessage);
      window.clearTimeout(timer);
    };
  }, []);

  // ---- Profile load ------------------------------------------------------
  const load = useCallback(async (accessToken: string) => {
    try {
      const r = await post<ProfileResponse>("HostedGetProfile", { clientId, accessToken });
      setProfile(r);
      if (!r.success) setError(r.message || "Not signed in.");
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to load your profile.");
    }
  }, [clientId]);

  useEffect(() => {
    if (token) void load(token);
  }, [token, load]);

  // ---- Actions -----------------------------------------------------------
  const startEdit = () => {
    setError(null);
    setNotice(null);
    setEditUsername(profile?.user?.username ?? "");
    setEditEmail(profile?.user?.email ?? "");
    setEditing(true);
  };

  const saveProfile = async (e: FormEvent) => {
    e.preventDefault();
    setError(null);
    setNotice(null);
    setSaveBusy(true);
    try {
      // Send only changed fields; the server rejects a locked field with a
      // human-readable message we surface as-is.
      const r = await post<ProfileResponse>("HostedUpdateProfile", {
        clientId,
        accessToken: token,
        ...(editUsername.trim() !== (profile?.user?.username ?? "")
          ? { username: editUsername.trim() }
          : {}),
        ...(editEmail.trim() !== (profile?.user?.email ?? "")
          ? { email: editEmail.trim() }
          : {}),
      });
      if (!r.success) throw new Error(r.message || "Could not update your profile.");
      setNotice(r.message || "Profile updated.");
      setEditing(false);
      if (r.user) setProfile((p) => (p ? { ...p, user: r.user } : p));
      else if (token) void load(token);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Could not update your profile.");
    } finally {
      setSaveBusy(false);
    }
  };

  const changePassword = async (e: FormEvent) => {
    e.preventDefault();
    setError(null);
    setNotice(null);
    if (newPw.length < 8) {
      setError("New password must be at least 8 characters long.");
      return;
    }
    if (newPw !== confirmPw) {
      setError("New passwords do not match.");
      return;
    }
    setPwBusy(true);
    try {
      const r = await post<SimpleResponse>("HostedChangePassword", {
        clientId,
        accessToken: token,
        currentPassword: currentPw,
        newPassword: newPw,
      });
      if (!r.success) throw new Error(r.message || "Password change failed.");
      setNotice("Password changed. Your other devices have been signed out.");
      setCurrentPw("");
      setNewPw("");
      setConfirmPw("");
      if (token) void load(token); // refresh the sessions list
    } catch (err) {
      setError(err instanceof Error ? err.message : "Password change failed.");
    } finally {
      setPwBusy(false);
    }
  };

  const revokeSession = async (sessionId: string) => {
    setError(null);
    setNotice(null);
    try {
      const r = await post<SimpleResponse>("HostedRevokeSession", {
        clientId,
        accessToken: token,
        sessionId,
      });
      if (!r.success) throw new Error(r.message || "Failed to revoke the session.");
      setNotice("Session revoked.");
      if (token) void load(token);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to revoke the session.");
    }
  };

  const logoutEverywhere = async () => {
    setError(null);
    setNotice(null);
    try {
      const r = await post<SimpleResponse>("HostedLogoutAll", { clientId, accessToken: token });
      if (!r.success) throw new Error(r.message || "Sign-out failed.");
      setLoggedOut(true);
      if (window.parent !== window) {
        // Tell the host app so it can clear its cookie and redirect. Scoped
        // to the verified handshake origin when we have one.
        window.parent.postMessage({ type: "LOGGED_OUT" }, parentOrigin.current ?? "*");
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : "Sign-out failed.");
    }
  };

  // ---- Render ------------------------------------------------------------
  const shell = (body: JSX.Element) => (
    <div style={{ minHeight: "100vh", display: "grid", placeItems: "center", padding: 16 }}>
      <div className="card" style={{ width: "100%", maxWidth: 460 }}>{body}</div>
    </div>
  );

  if (loggedOut) {
    return shell(
      <>
        <h2>Signed out</h2>
        <p className="card-sub">You have been signed out on every device.</p>
      </>
    );
  }

  if (token === null) {
    return shell(<div className="empty">Loading your account…</div>);
  }

  if (token === "" || (profile !== null && !profile.success)) {
    return shell(
      <>
        <h2>Not signed in</h2>
        <p className="card-sub">
          We couldn't find an active session. Sign in to the app first, then
          open your account page again.
        </p>
      </>
    );
  }

  if (profile === null) {
    return shell(<div className="empty">Loading your account…</div>);
  }

  const sessions = profile.sessions ?? [];

  return shell(
    <>
      <h2>Your account</h2>
      <p className="card-sub">Account management hosted by kaplabs iam.</p>

      {editing ? (
        <form onSubmit={saveProfile} style={{ margin: "12px 0" }}>
          <label className="field">
            <span>Username</span>
            <input
              type="text"
              value={editUsername}
              onChange={(e) => setEditUsername(e.target.value)}
              autoComplete="username"
            />
          </label>
          <label className="field">
            <span>Email</span>
            <input
              type="email"
              value={editEmail}
              onChange={(e) => setEditEmail(e.target.value)}
              autoComplete="email"
            />
          </label>
          <div style={{ display: "flex", gap: 8 }}>
            <button className="btn" disabled={saveBusy}>
              {saveBusy ? "Saving…" : "Save"}
            </button>
            <button
              type="button"
              className="btn secondary"
              disabled={saveBusy}
              onClick={() => {
                setEditing(false);
                setError(null);
              }}
            >
              Cancel
            </button>
          </div>
        </form>
      ) : (
        <div
          style={{
            margin: "12px 0",
            display: "flex",
            alignItems: "center",
            justifyContent: "space-between",
            gap: 8,
          }}
        >
          <div style={{ minWidth: 0 }}>
            <div><strong>{profile.user?.username || "—"}</strong></div>
            <div className="card-sub">{profile.user?.email}</div>
          </div>
          <button className="btn secondary" style={{ flexShrink: 0 }} onClick={startEdit}>
            Edit
          </button>
        </div>
      )}

      {notice && <p style={{ color: "var(--allow, #2e7d32)", fontSize: 13 }}>{notice}</p>}
      {error && <p className="error-text">{error}</p>}

      <h3 style={{ marginTop: 18 }}>Active sessions</h3>
      {sessions.length === 0 ? (
        <div className="empty">No active sessions.</div>
      ) : (
        <ul style={{ listStyle: "none", padding: 0, margin: 0 }}>
          {sessions.map((s) => (
            <li
              key={s.sessionId}
              style={{
                display: "flex",
                alignItems: "center",
                justifyContent: "space-between",
                gap: 8,
                padding: "8px 0",
                borderBottom: "1px solid rgba(128,128,128,0.2)",
              }}
            >
              <div style={{ minWidth: 0 }}>
                <div style={{ fontSize: 13, overflow: "hidden", textOverflow: "ellipsis" }}>
                  {shortAgent(s.userAgent)}
                  {s.current && (
                    <span className="badge active" style={{ marginLeft: 8 }}>
                      current
                    </span>
                  )}
                </div>
                <div className="card-sub" style={{ fontSize: 12 }}>
                  Signed in {formatDate(s.createdAt)}
                </div>
              </div>
              <button
                className="btn"
                style={{ flexShrink: 0 }}
                onClick={() => void revokeSession(s.sessionId)}
              >
                Revoke
              </button>
            </li>
          ))}
        </ul>
      )}

      <h3 style={{ marginTop: 18 }}>Change password</h3>
      <form onSubmit={changePassword}>
        <label className="field">
          <span>Current password</span>
          <input
            type="password"
            value={currentPw}
            onChange={(e) => setCurrentPw(e.target.value)}
            autoComplete="current-password"
            required
          />
        </label>
        <label className="field">
          <span>New password (min 8 characters)</span>
          <input
            type="password"
            value={newPw}
            onChange={(e) => setNewPw(e.target.value)}
            autoComplete="new-password"
            required
          />
        </label>
        <label className="field">
          <span>Confirm new password</span>
          <input
            type="password"
            value={confirmPw}
            onChange={(e) => setConfirmPw(e.target.value)}
            autoComplete="new-password"
            required
          />
        </label>
        <div style={{ marginTop: 12 }}>
          <button className="btn" disabled={pwBusy} style={{ width: "100%" }}>
            {pwBusy ? "Changing…" : "Change password"}
          </button>
        </div>
      </form>

      <div style={{ marginTop: 18 }}>
        <button className="btn danger" style={{ width: "100%" }} onClick={() => void logoutEverywhere()}>
          Sign out everywhere
        </button>
      </div>
    </>
  );
}
