import { useEffect, useState } from "react";
import { api, Developer, Organization } from "../../api";

export default function Developers() {
  const [devs, setDevs] = useState<Developer[] | null>(null);
  const [orgs, setOrgs] = useState<Organization[]>([]);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    Promise.all([api.listDevelopers(), api.listAllOrgs()])
      .then(([d, o]) => {
        setDevs(d);
        setOrgs(o);
      })
      .catch((e) => setError(e instanceof Error ? e.message : "Failed to load."));
  }, []);

  if (error) return <div className="empty">{error}</div>;
  if (!devs) return <div className="empty">Loading…</div>;

  const orgName = (id: string) => orgs.find((o) => o.org_id === id)?.name ?? id;

  return (
    <>
      <div className="page-eyebrow">platform</div>
      <div className="page-head">
        <div>
          <h1>Developers</h1>
          <p className="page-sub">
            Platform accounts. Superadmin is not a flag here — it's a tuple on the
            platform client (app:platform admin user:&lt;dev&gt;).
          </p>
        </div>
      </div>

      <div className="card" style={{ padding: 0 }}>
        <table className="data">
          <thead>
            <tr>
              <th>Email</th>
              <th>developer_id</th>
              <th>Org</th>
              <th>Joined</th>
            </tr>
          </thead>
          <tbody>
            {devs.map((d) => (
              <tr key={d.developer_id}>
                <td style={{ fontWeight: 600 }}>{d.email}</td>
                <td>
                  <code className="idchip">{d.developer_id}</code>
                </td>
                <td>{orgName(d.org_id)}</td>
                <td style={{ color: "var(--muted)" }}>{d.created_at.slice(0, 10)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </>
  );
}
