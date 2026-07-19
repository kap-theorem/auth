import { FormEvent, useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import { api, App, IdentityScope } from "../api";
import { useAuth } from "../auth";
import SecretModal from "../components/SecretModal";

export default function Apps() {
  const { session } = useAuth();
  const navigate = useNavigate();
  const [apps, setApps] = useState<App[] | null>(null);
  const [creating, setCreating] = useState(false);
  const [name, setName] = useState("");
  const [scope, setScope] = useState<IdentityScope>("app");
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [secret, setSecret] = useState<{ clientId: string; value: string } | null>(null);

  const load = () => api.listApps().then(setApps);
  useEffect(() => {
    load();
  }, []);

  const create = async (e: FormEvent) => {
    e.preventDefault();
    setError(null);
    setBusy(true);
    try {
      const { app, client_secret } = await api.createApp(name, scope);
      setSecret({ clientId: app.client_id, value: client_secret });
      setName("");
      setScope("app");
      setCreating(false);
      await load();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Could not create the app.");
    } finally {
      setBusy(false);
    }
  };

  return (
    <>
      <div className="page-eyebrow">{session?.org.name}</div>
      <div className="page-head">
        <div>
          <h1>Apps</h1>
          <p className="page-sub">Each app is a client of the platform — its own credentials, model, and tuples.</p>
        </div>
        <button className="btn" onClick={() => setCreating((v) => !v)}>
          {creating ? "Cancel" : "Create app"}
        </button>
      </div>

      {creating && (
        <form className="card" onSubmit={create} style={{ marginBottom: 16 }}>
          <h2>New app</h2>
          <p className="card-sub">The client secret is shown once after creation.</p>
          <div className="form-row">
            <label className="field">
              <span>Name</span>
              <input type="text" value={name} onChange={(e) => setName(e.target.value)} placeholder="wordskali" required />
            </label>
            <label className="field">
              <span>Identity scope</span>
              <select className="mono" value={scope} onChange={(e) => setScope(e.target.value as IdentityScope)}>
                <option value="app">app — isolated user base</option>
                <option value="org">org — shared org users (SSO)</option>
              </select>
            </label>
          </div>
          {error && <p className="error-text">{error}</p>}
          <button className="btn" disabled={busy}>
            {busy ? "Creating…" : "Create app"}
          </button>
        </form>
      )}

      <div className="card" style={{ padding: 0 }}>
        {apps === null ? (
          <div className="empty">Loading…</div>
        ) : apps.length === 0 ? (
          <div className="empty">
            No apps yet. Create one to get a client_id and secret.
          </div>
        ) : (
          <table className="data">
            <thead>
              <tr>
                <th>Name</th>
                <th>client_id</th>
                <th>Identity scope</th>
                <th>Status</th>
                <th>Created</th>
              </tr>
            </thead>
            <tbody>
              {apps.map((a) => (
                <tr
                  key={a.client_id}
                  className="rowlink"
                  tabIndex={0}
                  onClick={() => navigate(`/apps/${a.client_id}`)}
                  onKeyDown={(e) => e.key === "Enter" && navigate(`/apps/${a.client_id}`)}
                >
                  <td style={{ fontWeight: 600 }}>{a.name}</td>
                  <td>
                    <code className="idchip">{a.client_id}</code>
                  </td>
                  <td>
                    <span className="badge">{a.identity_scope}</span>
                  </td>
                  <td>
                    <span className={`badge ${a.status}`}>{a.status}</span>
                  </td>
                  <td style={{ color: "var(--muted)" }}>{a.created_at.slice(0, 10)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>

      {secret && (
        <SecretModal
          title="App created — save your client secret"
          clientId={secret.clientId}
          secret={secret.value}
          onClose={() => setSecret(null)}
        />
      )}
    </>
  );
}
