import { FormEvent, useEffect, useState } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";
import { api, App, AuthzModel, Effect, IdentityScope, RelationTuple, SubjectType } from "../api";
import SecretModal from "../components/SecretModal";
import Fact from "../components/Fact";

type Tab = "credentials" | "model" | "tuples" | "check";

export default function AppDetail() {
  const { clientId = "" } = useParams();
  const [app, setApp] = useState<App | null>(null);
  const [missing, setMissing] = useState(false);
  const [tab, setTab] = useState<Tab>("credentials");

  useEffect(() => {
    api
      .listApps()
      .then((apps) => {
        const found = apps.find((a) => a.client_id === clientId) ?? null;
        setApp(found);
        setMissing(!found);
      })
      .catch(() => setMissing(true));
  }, [clientId]);

  if (missing)
    return (
      <div className="empty">
        This app doesn't exist in your org. <Link to="/">Back to apps</Link>
      </div>
    );
  if (!app) return <div className="empty">Loading…</div>;

  return (
    <>
      <div className="page-eyebrow">
        <Link to="/" style={{ color: "inherit" }}>
          apps
        </Link>{" "}
        / {app.client_id}
      </div>
      <div className="page-head">
        <div>
          <h1>{app.name}</h1>
          <p className="page-sub">
            <span className={`badge ${app.status}`}>{app.status}</span>{" "}
            <span className="badge">identity: {app.identity_scope}</span>
          </p>
        </div>
      </div>

      <div className="tabs" role="tablist">
        {(
          [
            ["credentials", "Credentials"],
            ["model", "Authz model"],
            ["tuples", "Tuples"],
            ["check", "Check tester"],
          ] as [Tab, string][]
        ).map(([id, label]) => (
          <button
            key={id}
            role="tab"
            aria-selected={tab === id}
            className={"tab" + (tab === id ? " active" : "")}
            onClick={() => setTab(id)}
          >
            {label}
          </button>
        ))}
      </div>

      {tab === "credentials" && <CredentialsTab app={app} onChange={setApp} />}
      {tab === "model" && <ModelTab clientId={app.client_id} />}
      {tab === "tuples" && <TuplesTab clientId={app.client_id} />}
      {tab === "check" && <CheckTab clientId={app.client_id} />}
    </>
  );
}

// ---------------------------------------------------------- credentials

function CredentialsTab({ app, onChange }: { app: App; onChange: (a: App) => void }) {
  const navigate = useNavigate();
  const [name, setName] = useState(app.name);
  const [error, setError] = useState<string | null>(null);
  const [saved, setSaved] = useState(false);
  const [busy, setBusy] = useState(false);
  const [secret, setSecret] = useState<string | null>(null);
  const [confirmDelete, setConfirmDelete] = useState(false);

  const run = async (fn: () => Promise<void>) => {
    setError(null);
    setSaved(false);
    setBusy(true);
    try {
      await fn();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Request failed.");
    } finally {
      setBusy(false);
    }
  };

  const saveName = (e: FormEvent) => {
    e.preventDefault();
    run(async () => {
      onChange(await api.updateApp(app.client_id, { name }));
      setSaved(true);
    });
  };

  const setScope = (scope: IdentityScope) =>
    run(async () => {
      onChange(await api.updateApp(app.client_id, { identity_scope: scope }));
    });

  const rotate = () =>
    run(async () => {
      const { client_secret } = await api.rotateAppSecret(app.client_id);
      setSecret(client_secret);
    });

  const saveRedirects = (uris: string[]) =>
    run(async () => {
      onChange(await api.updateApp(app.client_id, { redirect_uris: uris }));
    });

  const remove = () =>
    run(async () => {
      await api.deleteApp(app.client_id);
      navigate("/");
    });

  return (
    <>
      <div className="card">
        <h2>Credentials</h2>
        <p className="card-sub">
          Your app authenticates every RPC with this pair. The secret is stored hashed —
          it can only be replaced, never read back.
        </p>
        <p>
          client_id <code className="idchip">{app.client_id}</code>
        </p>
        <p>
          client_secret <code className="idchip">bcrypt hash on file — not retrievable</code>
        </p>
        <button className="btn secondary" onClick={rotate} disabled={busy}>
          Rotate secret
        </button>
        <p className="hint">Rotation invalidates the old secret immediately; the new one is shown once.</p>
      </div>

      <RedirectUrisCard app={app} busy={busy} onSave={saveRedirects} />

      <form className="card" onSubmit={saveName}>
        <h2>Settings</h2>
        <label className="field" style={{ maxWidth: 360 }}>
          <span>App name</span>
          <input type="text" value={name} onChange={(e) => setName(e.target.value)} required />
        </label>
        <button className="btn secondary" disabled={busy || name === app.name}>
          Save name
        </button>
        {saved && <p className="ok-text">Saved.</p>}

        <div style={{ marginTop: 20 }}>
          <label className="field" style={{ maxWidth: 360 }}>
            <span>Identity scope</span>
            <select
              className="mono"
              value={app.identity_scope}
              onChange={(e) => setScope(e.target.value as IdentityScope)}
              disabled={busy}
            >
              <option value="app">app — isolated user base</option>
              <option value="org">org — shared org users (SSO)</option>
            </select>
          </label>
          <p className="hint">
            org scope shares who your users are across the org's apps — never what they
            can do. Tuples stay per-app either way.
          </p>
        </div>
      </form>

      <div className="card">
        <h2>Delete app</h2>
        <p className="card-sub">Removes the client, its authz model, and all its tuples. There is no undo.</p>
        {confirmDelete ? (
          <>
            <p className="error-text" style={{ marginBottom: 10 }}>
              Delete <strong>{app.name}</strong> and everything under it?
            </p>
            <button className="btn danger" onClick={remove} disabled={busy}>
              Yes, delete permanently
            </button>{" "}
            <button className="btn secondary" onClick={() => setConfirmDelete(false)}>
              Keep it
            </button>
          </>
        ) : (
          <button className="btn danger" onClick={() => setConfirmDelete(true)}>
            Delete app
          </button>
        )}
      </div>

      {error && <p className="error-text">{error}</p>}

      {secret && (
        <SecretModal
          title="Secret rotated — save the new client secret"
          clientId={app.client_id}
          secret={secret}
          onClose={() => setSecret(null)}
        />
      )}
    </>
  );
}

// ---------------------------------------------------- hosted login redirects

function RedirectUrisCard({
  app,
  busy,
  onSave,
}: {
  app: App;
  busy: boolean;
  onSave: (uris: string[]) => void;
}) {
  const [draft, setDraft] = useState("");
  const [draftError, setDraftError] = useState<string | null>(null);
  const uris = app.redirect_uris;

  const add = (e: FormEvent) => {
    e.preventDefault();
    setDraftError(null);
    const uri = draft.trim();
    let origin: URL;
    try {
      origin = new URL(uri);
    } catch {
      setDraftError("Enter an absolute URL, e.g. https://app.example.com/auth/callback.");
      return;
    }
    if (!origin.protocol || !origin.host) {
      setDraftError("Enter an absolute URL, e.g. https://app.example.com/auth/callback.");
      return;
    }
    if (uris.includes(uri)) {
      setDraftError("That URI is already whitelisted.");
      return;
    }
    setDraft("");
    onSave([...uris, uri]);
  };

  const remove = (uri: string) => onSave(uris.filter((u) => u !== uri));

  return (
    <div className="card">
      <h2>Hosted login</h2>
      <p className="card-sub">
        Whitelist the exact redirect URIs allowed to use the platform-hosted login page
        at <code>/client/{app.client_id}/user/login</code>. Matching is exact string
        equality — no prefixes, no wildcards. An empty list disables hosted login.
      </p>
      {uris.length === 0 ? (
        <p className="hint">No redirect URIs — hosted login is disabled for this app.</p>
      ) : (
        <ul style={{ listStyle: "none", padding: 0, margin: "0 0 12px" }}>
          {uris.map((uri) => (
            <li
              key={uri}
              style={{ display: "flex", alignItems: "center", gap: 10, marginBottom: 6 }}
            >
              <code className="idchip" style={{ flex: 1, overflowWrap: "anywhere" }}>
                {uri}
              </code>
              <button
                type="button"
                className="btn danger small"
                onClick={() => remove(uri)}
                disabled={busy}
              >
                Remove
              </button>
            </li>
          ))}
        </ul>
      )}
      <form onSubmit={add} style={{ display: "flex", gap: 10, alignItems: "flex-start" }}>
        <label className="field" style={{ flex: 1, maxWidth: 420 }}>
          <span>Add redirect URI</span>
          <input
            type="text"
            className="mono"
            placeholder="https://app.example.com/auth/callback"
            value={draft}
            onChange={(e) => setDraft(e.target.value)}
            required
          />
        </label>
        <button className="btn secondary" disabled={busy} style={{ marginTop: 22 }}>
          Add
        </button>
      </form>
      {draftError && <p className="error-text">{draftError}</p>}
    </div>
  );
}

// ---------------------------------------------------------- authz model

const MODEL_TEMPLATE = JSON.stringify(
  {
    types: {
      app: { relations: { admin: [], member: ["admin"] } },
      document: { relations: { owner: [], viewer: ["owner"] } },
    },
  },
  null,
  2
);

function ModelTab({ clientId }: { clientId: string }) {
  const [model, setModel] = useState<AuthzModel | null>(null);
  const [text, setText] = useState("");
  const [loaded, setLoaded] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [savedVersion, setSavedVersion] = useState<number | null>(null);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    api.getAuthzModel(clientId).then((m) => {
      setModel(m);
      setText(m?.model_json ?? MODEL_TEMPLATE);
      setLoaded(true);
    });
  }, [clientId]);

  // live validation
  let parseError: string | null = null;
  try {
    JSON.parse(text);
  } catch (e) {
    parseError = e instanceof Error ? e.message : "Invalid JSON";
  }

  const save = async () => {
    setError(null);
    setSavedVersion(null);
    setBusy(true);
    try {
      const next = await api.writeAuthzModel(clientId, text);
      setModel(next);
      setSavedVersion(next.version);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Could not save the model.");
    } finally {
      setBusy(false);
    }
  };

  if (!loaded) return <div className="empty">Loading…</div>;

  return (
    <div className="card">
      <h2>Authorization model</h2>
      <p className="card-sub">
        Declare your object types and relations. A relation lists the relations that
        imply it — e.g. <code>"viewer": ["editor"]</code> means editors can do whatever
        viewers can. Checks always evaluate the latest version.
      </p>
      <p className="hint" style={{ marginBottom: 8 }}>
        {model ? (
          <>
            version <code className="idchip">v{model.version}</code> · updated {model.updated_at.slice(0, 10)}
          </>
        ) : (
          "No model yet — the template below is a starting point."
        )}
      </p>
      <textarea
        rows={16}
        value={text}
        onChange={(e) => setText(e.target.value)}
        spellCheck={false}
        aria-label="Model JSON"
      />
      {parseError ? (
        <p className="error-text">JSON: {parseError}</p>
      ) : (
        <p className="ok-text">Valid JSON.</p>
      )}
      {error && <p className="error-text">{error}</p>}
      {savedVersion !== null && <p className="ok-text">Saved as version v{savedVersion}.</p>}
      <div style={{ marginTop: 10 }}>
        <button className="btn" onClick={save} disabled={busy || !!parseError}>
          {busy ? "Saving…" : model ? `Save as v${model.version + 1}` : "Save as v1"}
        </button>
      </div>
    </div>
  );
}

// ---------------------------------------------------------- tuples

const EMPTY_TUPLE = {
  object: "",
  relation: "",
  subject_type: "user" as SubjectType,
  subject_id: "",
  effect: "allow" as Effect,
  condition_expr: "",
};

function TuplesTab({ clientId }: { clientId: string }) {
  const [rows, setRows] = useState<RelationTuple[] | null>(null);
  const [objectFilter, setObjectFilter] = useState("");
  const [subjectFilter, setSubjectFilter] = useState("");
  const [draft, setDraft] = useState(EMPTY_TUPLE);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  const load = () =>
    api
      .listTuples(clientId, { object: objectFilter || undefined, subject: subjectFilter || undefined })
      .then(setRows);

  useEffect(() => {
    load();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [clientId, objectFilter, subjectFilter]);

  const add = async (e: FormEvent) => {
    e.preventDefault();
    setError(null);
    const colon = draft.object.indexOf(":");
    if (colon <= 0 || colon === draft.object.length - 1) {
      setError("Object must look like type:id, e.g. problem:two-sum.");
      return;
    }
    setBusy(true);
    try {
      await api.writeTuples(clientId, [
        {
          object_type: draft.object.slice(0, colon).trim(),
          object_id: draft.object.slice(colon + 1).trim(),
          relation: draft.relation.trim(),
          subject_type: draft.subject_type,
          subject_id: draft.subject_id.trim(),
          effect: draft.effect,
          condition_expr: draft.condition_expr.trim() || undefined,
        },
      ]);
      setDraft(EMPTY_TUPLE);
      await load();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Could not write the tuple.");
    } finally {
      setBusy(false);
    }
  };

  const remove = async (row: RelationTuple) => {
    setBusy(true);
    try {
      const { created_at: _ca, ...key } = row;
      await api.deleteTuples(clientId, [key]);
      await load();
    } finally {
      setBusy(false);
    }
  };

  return (
    <>
      <form className="card" onSubmit={add}>
        <h2>Add tuple</h2>
        <p className="card-sub">One tuple is one access fact. Deny always wins over allow.</p>
        <div className="form-row">
          <label className="field">
            <span>Object (type:id)</span>
            <input
              type="text"
              className="mono"
              placeholder="problem:two-sum"
              value={draft.object}
              onChange={(e) => setDraft({ ...draft, object: e.target.value })}
              required
            />
          </label>
          <label className="field">
            <span>Relation</span>
            <input
              type="text"
              className="mono"
              placeholder="editor"
              value={draft.relation}
              onChange={(e) => setDraft({ ...draft, relation: e.target.value })}
              required
            />
          </label>
        </div>
        <div className="form-row">
          <label className="field">
            <span>Subject type</span>
            <select
              className="mono"
              value={draft.subject_type}
              onChange={(e) => setDraft({ ...draft, subject_type: e.target.value as SubjectType })}
            >
              <option value="user">user</option>
              <option value="role">role</option>
            </select>
          </label>
          <label className="field">
            <span>Subject id</span>
            <input
              type="text"
              className="mono"
              placeholder="kush"
              value={draft.subject_id}
              onChange={(e) => setDraft({ ...draft, subject_id: e.target.value })}
              required
            />
          </label>
          <label className="field">
            <span>Effect</span>
            <select
              className="mono"
              value={draft.effect}
              onChange={(e) => setDraft({ ...draft, effect: e.target.value as Effect })}
            >
              <option value="allow">allow</option>
              <option value="deny">deny</option>
            </select>
          </label>
        </div>
        <label className="field">
          <span>Condition (optional ABAC expression)</span>
          <input
            type="text"
            className="mono"
            placeholder='env == "staging"'
            value={draft.condition_expr}
            onChange={(e) => setDraft({ ...draft, condition_expr: e.target.value })}
          />
          <span className="hint">
            Grammar: comparisons <code>==</code> <code>!=</code> <code>&lt;</code>{" "}
            <code>&gt;</code>, joined with <code>&amp;&amp;</code> / <code>||</code>;{" "}
            <code>now()</code>, context keys as identifiers, string literals in double
            quotes — e.g. <code>env == "staging" &amp;&amp; now() &lt;
            "2026-08-01T00:00:00Z"</code>
          </span>
        </label>
        {error && <p className="error-text">{error}</p>}
        <button className="btn" disabled={busy}>
          Add tuple
        </button>
      </form>

      <div className="card" style={{ padding: 0 }}>
        <div style={{ display: "flex", gap: 10, padding: "14px 16px", borderBottom: "1px solid var(--line)" }}>
          <input
            type="text"
            className="mono"
            placeholder="Filter by object…"
            value={objectFilter}
            onChange={(e) => setObjectFilter(e.target.value)}
            aria-label="Filter by object"
          />
          <input
            type="text"
            className="mono"
            placeholder="Filter by subject…"
            value={subjectFilter}
            onChange={(e) => setSubjectFilter(e.target.value)}
            aria-label="Filter by subject"
          />
        </div>
        {rows === null ? (
          <div className="empty">Loading…</div>
        ) : rows.length === 0 ? (
          <div className="empty">No tuples match. An empty store means every check denies.</div>
        ) : (
          <table className="data">
            <thead>
              <tr>
                <th>Fact</th>
                <th>Effect</th>
                <th>Condition</th>
                <th></th>
              </tr>
            </thead>
            <tbody>
              {rows.map((r, i) => (
                <tr key={i}>
                  <td>
                    <Fact tuple={r} />
                  </td>
                  <td>
                    <span className={`badge ${r.effect}`}>{r.effect}</span>
                  </td>
                  <td>
                    {r.condition_expr ? <code className="idchip">{r.condition_expr}</code> : <span style={{ color: "var(--muted)" }}>—</span>}
                  </td>
                  <td style={{ textAlign: "right" }}>
                    <button className="btn danger small" onClick={() => remove(r)} disabled={busy}>
                      Delete
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>
    </>
  );
}

// ---------------------------------------------------------- check tester

function CheckTab({ clientId }: { clientId: string }) {
  const [subject, setSubject] = useState("user:");
  const [relation, setRelation] = useState("");
  const [object, setObject] = useState("");
  const [contextText, setContextText] = useState("");
  const [result, setResult] = useState<{ allowed: boolean; reason?: string } | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  const run = async (e: FormEvent) => {
    e.preventDefault();
    setError(null);
    setResult(null);

    let context: Record<string, string> = {};
    if (contextText.trim()) {
      try {
        const parsed = JSON.parse(contextText);
        if (typeof parsed !== "object" || parsed === null || Array.isArray(parsed)) throw new Error();
        context = Object.fromEntries(Object.entries(parsed).map(([k, v]) => [k, String(v)]));
      } catch {
        setError('Context must be a JSON object, e.g. {"env": "staging"}.');
        return;
      }
    }

    setBusy(true);
    try {
      setResult(await api.check(clientId, subject.trim(), relation.trim(), object.trim(), context));
    } catch (err) {
      setError(err instanceof Error ? err.message : "Check failed.");
    } finally {
      setBusy(false);
    }
  };

  return (
    <form className="card" onSubmit={run}>
      <h2>Check tester</h2>
      <p className="card-sub">
        Runs a real Check against this app's live tuples and model — the same call your
        backend makes. Default deny: no matching tuple means denied.
      </p>
      <div className="form-row">
        <label className="field">
          <span>Subject</span>
          <input
            type="text"
            className="mono"
            placeholder="user:kush"
            value={subject}
            onChange={(e) => setSubject(e.target.value)}
            required
          />
        </label>
        <label className="field">
          <span>Relation</span>
          <input
            type="text"
            className="mono"
            placeholder="viewer"
            value={relation}
            onChange={(e) => setRelation(e.target.value)}
            required
          />
        </label>
        <label className="field">
          <span>Object</span>
          <input
            type="text"
            className="mono"
            placeholder="problem:two-sum"
            value={object}
            onChange={(e) => setObject(e.target.value)}
            required
          />
        </label>
      </div>
      <label className="field">
        <span>Context (JSON, for conditions)</span>
        <input
          type="text"
          className="mono"
          placeholder='{"env": "staging"}'
          value={contextText}
          onChange={(e) => setContextText(e.target.value)}
        />
      </label>
      {error && <p className="error-text">{error}</p>}
      <button className="btn" disabled={busy}>
        {busy ? "Checking…" : "Run check"}
      </button>

      {result && (
        <div className={"verdict " + (result.allowed ? "allowed" : "denied")} role="status">
          <div className="word">{result.allowed ? "ALLOWED" : "DENIED"}</div>
          {result.reason && <div className="reason">{result.reason}</div>}
        </div>
      )}
    </form>
  );
}
