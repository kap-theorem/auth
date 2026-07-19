// Public hosted-login page (spec "Hosted login", added 2026-07-18).
//
// Served at /client/:clientId/user/login?redirect=<uri> with NO console
// session and minimal chrome, designed to run inside a consumer app's
// iframe. It always talks to the real backend directly (the vite proxy /
// same-origin gateway), regardless of VITE_API_MODE — the mock API has no
// end users.
//
// Sign-in surface is driven by GetAppPublicInfo:
//   - demoEnabled     → a one-click "Login as Demo User" button (HostedDemoLogin)
//   - publicSignup    → a "Sign up" form (HostedRegister); hidden otherwise
//   - loginIdentifier → the identifier field's label/placeholder
// Demo, signup and password login all deliver the token the same way.
//
// Token delivery on success:
//   - inside an iframe: postMessage({type: "AUTH_SUCCESS", access_token})
//     to window.parent with targetOrigin derived from the redirect param
//   - top-level: browser redirect to `<redirect>#access_token=<token>`

import { FormEvent, useEffect, useMemo, useState } from "react";
import { useParams } from "react-router-dom";

const BASE = "/auth.v1.PlatformService";

type LoginIdentifier = "username_or_email" | "email_only" | "username_only";

interface PublicInfo {
  success: boolean;
  name?: string;
  hostedLoginEnabled?: boolean;
  demoEnabled?: boolean;
  loginIdentifier?: LoginIdentifier;
  publicSignup?: boolean;
}

interface TokenResponse {
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

function identifierLabel(mode: LoginIdentifier | undefined): string {
  if (mode === "email_only") return "Email";
  if (mode === "username_only") return "Username";
  return "Email or Username";
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
  const [mode, setMode] = useState<"login" | "signup">("login");
  const [identifier, setIdentifier] = useState("");
  const [username, setUsername] = useState("");
  const [signupEmail, setSignupEmail] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [done, setDone] = useState(false);

  const loginIdentifier = info?.loginIdentifier;

  useEffect(() => {
    post<PublicInfo>("GetAppPublicInfo", { clientId })
      .then(setInfo)
      .catch(() => setInfoFailed(true));
  }, [clientId]);

  // Shared success path: hand the access token to the embedding app (iframe)
  // or to the top-level page via a URL fragment. Demo/signup/login all use it.
  const deliverToken = (accessToken: string) => {
    setDone(true);
    if (window.parent !== window) {
      // Iframe flow: the target origin comes from the (whitelist-verified)
      // redirect param, never "*", so no other frame can intercept the token.
      window.parent.postMessage(
        { type: "AUTH_SUCCESS", access_token: accessToken },
        redirectOrigin ?? redirect
      );
    } else {
      // Top-level flow: fragment redirect (the token never hits a server log —
      // fragments are not sent in requests).
      window.location.href = `${redirect}#access_token=${encodeURIComponent(accessToken)}`;
    }
  };

  // Runs an RPC that returns a token-bearing response, then delivers the token.
  const runAuth = async (fn: () => Promise<TokenResponse>, failMsg: string) => {
    setError(null);
    setBusy(true);
    try {
      const r = await fn();
      if (!r.success || !r.accessToken) throw new Error(r.message || failMsg);
      deliverToken(r.accessToken);
    } catch (err) {
      setError(err instanceof Error ? err.message : failMsg);
    } finally {
      setBusy(false);
    }
  };

  const submitLogin = (e: FormEvent) => {
    e.preventDefault();
    // email_only accepts an address (lowercased); username modes are sent
    // as typed. The backend interprets this identifier per loginIdentifier.
    const value =
      loginIdentifier === "email_only" ? identifier.trim().toLowerCase() : identifier.trim();
    void runAuth(
      () =>
        post<TokenResponse>("HostedLogin", {
          clientId,
          email: value,
          password,
          redirectUri: redirect,
          userAgent: navigator.userAgent,
        }),
      "Invalid credentials"
    );
  };

  const submitSignup = (e: FormEvent) => {
    e.preventDefault();
    void runAuth(
      () =>
        post<TokenResponse>("HostedRegister", {
          clientId,
          username: username.trim(),
          email: signupEmail.trim().toLowerCase(),
          password,
          redirectUri: redirect,
          userAgent: navigator.userAgent,
        }),
      "Sign-up failed."
    );
  };

  const demoLogin = () =>
    void runAuth(
      () =>
        post<TokenResponse>("HostedDemoLogin", {
          clientId,
          redirectUri: redirect,
          userAgent: navigator.userAgent,
        }),
      "Demo sign-in failed."
    );

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
        ) : mode === "signup" && info?.publicSignup ? (
          <form onSubmit={submitSignup}>
            <h2>Create your {info?.name} account</h2>
            <p className="card-sub">Secure sign-up hosted by kaplabs iam.</p>
            <label className="field">
              <span>Username</span>
              <input
                type="text"
                value={username}
                onChange={(e) => setUsername(e.target.value)}
                autoComplete="username"
                required
              />
            </label>
            <label className="field">
              <span>Email</span>
              <input
                type="email"
                value={signupEmail}
                onChange={(e) => setSignupEmail(e.target.value)}
                autoComplete="email"
                required
              />
            </label>
            <label className="field">
              <span>Password</span>
              <input
                type="password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                autoComplete="new-password"
                required
              />
            </label>
            {error && <p className="error-text">{error}</p>}
            <div style={{ marginTop: 14 }}>
              <button className="btn" disabled={busy} style={{ width: "100%" }}>
                {busy ? "Creating account…" : "Sign up"}
              </button>
            </div>
            <p className="login-toggle">
              Already have an account?{" "}
              <button
                type="button"
                onClick={() => {
                  setMode("login");
                  setError(null);
                }}
              >
                Sign in
              </button>
            </p>
          </form>
        ) : (
          <form onSubmit={submitLogin}>
            <h2>Sign in to {info?.name}</h2>
            <p className="card-sub">Secure sign-in hosted by kaplabs iam.</p>
            <label className="field">
              <span>{identifierLabel(loginIdentifier)}</span>
              <input
                type={loginIdentifier === "email_only" ? "email" : "text"}
                value={identifier}
                onChange={(e) => setIdentifier(e.target.value)}
                placeholder={identifierLabel(loginIdentifier)}
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
            {info?.demoEnabled && (
              <div style={{ marginTop: 10 }}>
                <button
                  type="button"
                  className="btn secondary"
                  disabled={busy}
                  style={{ width: "100%" }}
                  onClick={demoLogin}
                >
                  Login as Demo User
                </button>
              </div>
            )}
            {info?.publicSignup && (
              <p className="login-toggle">
                New here?{" "}
                <button
                  type="button"
                  onClick={() => {
                    setMode("signup");
                    setError(null);
                  }}
                >
                  Sign up
                </button>
              </p>
            )}
          </form>
        )}
      </div>
    </div>
  );
}
