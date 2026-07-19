// Public hosted-login page (spec "Hosted login", added 2026-07-18).
//
// Served at /client/:clientId/user/login?redirect=<uri> with NO console
// session and minimal chrome, designed to run inside a consumer app's
// iframe. It always talks to the real backend directly (the vite proxy /
// same-origin gateway), regardless of VITE_API_MODE — the mock API has no
// end users. `login_type` (e.g. ?login_type=both) is intentionally ignored:
// registration requires a client secret, which this page never holds, so
// only login is rendered.
//
// Token delivery on success:
//   - inside an iframe: postMessage({type: "AUTH_SUCCESS", access_token})
//     to window.parent with targetOrigin derived from the redirect param
//   - top-level: browser redirect to `<redirect>#access_token=<token>`

import { FormEvent, useEffect, useMemo, useState } from "react";
import { useParams } from "react-router-dom";

const BASE = "/auth.v1.PlatformService";

interface PublicInfo {
  success: boolean;
  name?: string;
  hostedLoginEnabled?: boolean;
}

interface HostedLoginResponse {
  success: boolean;
  message?: string;
  accessToken?: string;
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
    throw new Error("Cannot reach the login service. Please try again.");
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

export default function HostedLogin() {
  const { clientId = "" } = useParams();
  const redirect = useMemo(
    () => new URLSearchParams(window.location.search).get("redirect") ?? "",
    []
  );
  const redirectOrigin = useMemo(() => {
    try {
      return new URL(redirect).origin;
    } catch {
      return null;
    }
  }, [redirect]);

  const [info, setInfo] = useState<PublicInfo | null>(null);
  const [infoFailed, setInfoFailed] = useState(false);
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [done, setDone] = useState(false);

  useEffect(() => {
    post<PublicInfo>("GetAppPublicInfo", { clientId })
      .then(setInfo)
      .catch(() => setInfoFailed(true));
  }, [clientId]);

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    setError(null);
    setBusy(true);
    try {
      const r = await post<HostedLoginResponse>("HostedLogin", {
        clientId,
        email: email.trim().toLowerCase(),
        password,
        redirectUri: redirect,
        userAgent: navigator.userAgent,
      });
      if (!r.success || !r.accessToken) {
        throw new Error(r.message || "Invalid credentials");
      }
      setDone(true);
      if (window.parent !== window) {
        // Iframe flow: hand the token to the embedding app. The target
        // origin comes from the (whitelist-verified) redirect param, never
        // "*", so no other frame can intercept the token.
        window.parent.postMessage(
          { type: "AUTH_SUCCESS", access_token: r.accessToken },
          redirectOrigin ?? redirect
        );
      } else {
        // Top-level flow: fragment redirect (the token never hits a server
        // log — fragments are not sent in requests).
        window.location.href = `${redirect}#access_token=${encodeURIComponent(r.accessToken)}`;
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : "Sign-in failed.");
    } finally {
      setBusy(false);
    }
  };

  const unavailable =
    !redirect ||
    !redirectOrigin ||
    infoFailed ||
    (info !== null && (!info.success || !info.hostedLoginEnabled));

  return (
    <div
      style={{
        minHeight: "100vh",
        display: "grid",
        placeItems: "center",
        padding: 16,
      }}
    >
      <div className="card" style={{ width: "100%", maxWidth: 380 }}>
        {info === null && !infoFailed ? (
          <div className="empty">Loading…</div>
        ) : unavailable ? (
          <>
            <h2>Sign-in unavailable</h2>
            <p className="card-sub">
              Hosted login is not available for this app. Check the link you
              followed, or contact the app's developer.
            </p>
          </>
        ) : done ? (
          <>
            <h2>Signed in</h2>
            <p className="card-sub">You can return to {info?.name}.</p>
          </>
        ) : (
          <form onSubmit={submit}>
            <h2>Sign in to {info?.name}</h2>
            <p className="card-sub">Secure sign-in hosted by kaplabs iam.</p>
            <label className="field">
              <span>Email</span>
              <input
                type="email"
                value={email}
                onChange={(e) => setEmail(e.target.value)}
                autoComplete="username"
                required
              />
            </label>
            <label className="field">
              <span>Password</span>
              <input
                type="password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                autoComplete="current-password"
                required
              />
            </label>
            {error && <p className="error-text">{error}</p>}
            <div style={{ marginTop: 14 }}>
              <button className="btn" disabled={busy} style={{ width: "100%" }}>
                {busy ? "Signing in…" : "Sign in"}
              </button>
            </div>
          </form>
        )}
      </div>
    </div>
  );
}
