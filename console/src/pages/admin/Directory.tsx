import { useEffect, useState } from "react";
import { api, App, Organization, PLATFORM_CLIENT_ID } from "../../api";

export default function Directory() {
  const [orgs, setOrgs] = useState<Organization[] | null>(null);
  const [apps, setApps] = useState<App[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busyId, setBusyId] = useState<string | null>(null);

  const load = () =>
    Promise.all([api.listAllOrgs(), api.listAllApps()])
      .then(([o, a]) => {
        setOrgs(o);
        setApps(a);
      })
      .catch((e) => setError(e instanceof Error ? e.message : "Failed to load."));

  useEffect(() => {
    load();
  }, []);

  const toggle = async (app: App) => {
    setBusyId(app.client_id);
    setError(null);
    try {
      if (app.status === "active") await api.suspendClient(app.client_id);
      else await api.restoreClient(app.client_id);
      await load();
    } catch (e) {
      setError(e instanceof Error ? e.message : "Request failed.");
    } finally {
      setBusyId(null);
    }
  };

  if (error && !orgs) return <div className="empty">{error}</div>;
  if (!orgs || !apps) return <div className="empty">Loading…</div>;

  return (
    <>
      <div className="page-eyebrow">platform</div>
      <div className="page-head">
        <div>
          <h1>Orgs &amp; apps</h1>
          <p className="page-sub">
            Every organization and registered client on the platform. Suspending a client
            rejects all of its API traffic immediately.
          </p>
        </div>
      </div>

      {error && <p className="error-text" style={{ marginBottom: 12 }}>{error}</p>}

      {orgs.map((org) => {
        const orgApps = apps.filter((a) => a.org_id === org.org_id);
        return (
          <div className="card" key={org.org_id} style={{ padding: 0 }}>
            <div style={{ padding: "14px 16px", borderBottom: "1px solid var(--line)" }}>
              <h2 style={{ fontSize: 14 }}>{org.name}</h2>
              <span className="hint">
                <code className="idchip">{org.org_id}</code> · {orgApps.length} app
                {orgApps.length === 1 ? "" : "s"}
              </span>
            </div>
            {orgApps.length === 0 ? (
              <div className="empty">No apps in this org.</div>
            ) : (
              <table className="data">
                <thead>
                  <tr>
                    <th>App</th>
                    <th>client_id</th>
                    <th>Scope</th>
                    <th>Status</th>
                    <th></th>
                  </tr>
                </thead>
                <tbody>
                  {orgApps.map((a) => (
                    <tr key={a.client_id}>
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
                      <td style={{ textAlign: "right" }}>
                        {a.client_id === PLATFORM_CLIENT_ID ? (
                          <span className="hint">built-in</span>
                        ) : (
                          <button
                            className={"btn small " + (a.status === "active" ? "danger" : "secondary")}
                            onClick={() => toggle(a)}
                            disabled={busyId === a.client_id}
                          >
                            {a.status === "active" ? "Suspend" : "Restore"}
                          </button>
                        )}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            )}
          </div>
        );
      })}
    </>
  );
}
