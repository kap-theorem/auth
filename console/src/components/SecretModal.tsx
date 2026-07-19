import { useState } from "react";

/** Show-once secret dialog: shown at app creation and secret rotation only. */
export default function SecretModal({
  title,
  clientId,
  secret,
  onClose,
}: {
  title: string;
  clientId: string;
  secret: string;
  onClose: () => void;
}) {
  const [copied, setCopied] = useState(false);

  const copy = async () => {
    try {
      await navigator.clipboard.writeText(secret);
      setCopied(true);
    } catch {
      // clipboard unavailable; the value is still selectable
    }
  };

  return (
    <div className="modal-backdrop" role="dialog" aria-modal="true" aria-label={title}>
      <div className="modal">
        <h2>{title}</h2>
        <p style={{ margin: 0, color: "var(--muted)" }}>
          This secret is shown once and stored only as a hash. Copy it now — closing this
          dialog is permanent.
        </p>
        <p className="hint">
          client_id <code className="idchip">{clientId}</code>
        </p>
        <div className="secret-box">{secret}</div>
        <div className="modal-actions">
          <button className="btn secondary" onClick={copy}>
            {copied ? "Copied" : "Copy secret"}
          </button>
          <button className="btn" onClick={onClose}>
            I saved it — close
          </button>
        </div>
      </div>
    </div>
  );
}
