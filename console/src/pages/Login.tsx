import { FormEvent, useState } from "react";
import { useAuth } from "../auth";

export default function Login() {
  const { signIn, register } = useAuth();
  const [mode, setMode] = useState<"login" | "register">("login");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    setError(null);
    setBusy(true);
    try {
      if (mode === "login") await signIn(email, password);
      else await register(email, password);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Something went wrong.");
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="login-wrap">
      <div className="login-left">
        <div className="wordmark">
          kaplabs <span>/</span> iam
        </div>
        <div className="login-tagline">
          <h1>Every access decision is a fact you can read.</h1>
          <p>
            One console for your apps' credentials, authorization models, and relation
            tuples — with a live Check tester for when a decision surprises you.
          </p>
        </div>
        <div className="login-facts" aria-hidden="true">
          <div>
            user:kush —author→ problem:two-sum <span className="allowc">· allow</span>
          </div>
          <div>
            role:moderator —editor→ problem:two-sum <span className="allowc">· allow</span>
          </div>
          <div>
            user:banned —⊘viewer→ problem:two-sum <span className="denyc">· deny</span>
          </div>
          <div>no matching tuple → default deny</div>
        </div>
      </div>

      <div className="login-right">
        <form className="login-card card" onSubmit={submit}>
          <h2>{mode === "login" ? "Sign in" : "Create a developer account"}</h2>
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
              autoComplete={mode === "login" ? "current-password" : "new-password"}
              required
            />
          </label>
          {mode === "register" && (
            <p className="hint">At least 8 characters. A personal org is created for you.</p>
          )}
          {error && <p className="error-text">{error}</p>}
          <div style={{ marginTop: 14 }}>
            <button className="btn" disabled={busy} style={{ width: "100%" }}>
              {busy ? "Working…" : mode === "login" ? "Sign in" : "Create account"}
            </button>
          </div>
          <p className="login-toggle">
            {mode === "login" ? (
              <>
                New here?{" "}
                <button type="button" onClick={() => { setMode("register"); setError(null); }}>
                  Create an account
                </button>
              </>
            ) : (
              <>
                Already registered?{" "}
                <button type="button" onClick={() => { setMode("login"); setError(null); }}>
                  Sign in
                </button>
              </>
            )}
          </p>
        </form>
      </div>
    </div>
  );
}
