import { useEffect, useState } from "react";
import { api, PlatformMetrics } from "../../api";

const fmt = new Intl.NumberFormat("en-US");

export default function Metrics() {
  const [m, setM] = useState<PlatformMetrics | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    api.getPlatformMetrics().then(setM).catch((e) =>
      setError(e instanceof Error ? e.message : "Failed to load.")
    );
  }, []);

  if (error) return <div className="empty">{error}</div>;
  if (!m) return <div className="empty">Loading…</div>;

  return (
    <>
      <div className="page-eyebrow">platform</div>
      <div className="page-head">
        <div>
          <h1>Metrics</h1>
          <p className="page-sub">Platform-wide counts.</p>
        </div>
      </div>

      <h2 style={{ fontSize: 13, color: "var(--muted)", margin: "0 0 10px" }}>Inventory</h2>
      <div className="tile-grid">
        <Tile label="Organizations" value={fmt.format(m.orgs)} />
        <Tile label="Apps" value={fmt.format(m.apps)} />
        <Tile label="Developers" value={fmt.format(m.developers)} />
      </div>

      <h2 style={{ fontSize: 13, color: "var(--muted)", margin: "22px 0 10px" }}>Usage</h2>
      <div className="tile-grid">
        <Tile label="End users" value={fmt.format(m.users)} />
        <Tile label="Active sessions" value={fmt.format(m.active_sessions)} />
        <Tile label="Relation tuples" value={fmt.format(m.tuples)} />
      </div>
    </>
  );
}

function Tile({ label, value }: { label: string; value: string }) {
  return (
    <div className="tile">
      <div className="tile-label">{label}</div>
      <div className="tile-value">{value}</div>
    </div>
  );
}
