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

  const denyRate = m.checks_24h > 0 ? m.check_denies_24h / m.checks_24h : 0;

  return (
    <>
      <div className="page-eyebrow">platform</div>
      <div className="page-head">
        <div>
          <h1>Metrics</h1>
          <p className="page-sub">Platform-wide activity over the last 24 hours.</p>
        </div>
      </div>

      <h2 style={{ fontSize: 13, color: "var(--muted)", margin: "0 0 10px" }}>Inventory</h2>
      <div className="tile-grid">
        <Tile label="Organizations" value={fmt.format(m.orgs)} />
        <Tile label="Apps" value={fmt.format(m.apps)} />
        <Tile label="Developers" value={fmt.format(m.developers)} />
        <Tile label="Active sessions" value={fmt.format(m.active_sessions)} />
      </div>

      <h2 style={{ fontSize: 13, color: "var(--muted)", margin: "22px 0 10px" }}>Last 24 hours</h2>
      <div className="tile-grid">
        <Tile label="Logins" value={fmt.format(m.logins_24h)} />
        <Tile label="Authz checks" value={fmt.format(m.checks_24h)} />
        <Tile
          label="Checks denied"
          value={fmt.format(m.check_denies_24h)}
          note={`${(denyRate * 100).toFixed(1)}% of checks`}
          tone={denyRate > 0.25 ? "deny" : undefined}
        />
        <Tile
          label="Error rate"
          value={`${(m.error_rate_24h * 100).toFixed(2)}%`}
          note="failed requests / all requests"
          tone={m.error_rate_24h > 0.01 ? "deny" : undefined}
        />
      </div>
    </>
  );
}

function Tile({
  label,
  value,
  note,
  tone,
}: {
  label: string;
  value: string;
  note?: string;
  tone?: "deny";
}) {
  return (
    <div className={"tile" + (tone === "deny" ? " deny-tone" : "")}>
      <div className="tile-label">{label}</div>
      <div className="tile-value">{value}</div>
      {note && <div className="tile-note">{note}</div>}
    </div>
  );
}
