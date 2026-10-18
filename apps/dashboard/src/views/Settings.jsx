import { useState } from "react";
import { Panel, Field, StatusPill } from "../components/ui";

export default function Settings({ apiBaseUrl, health, onSave }) {
  const [url, setUrl] = useState(apiBaseUrl);
  const [error, setError] = useState("");

  const save = (e) => {
    e.preventDefault();
    const value = url.trim().replace(/\/$/, "") || "/api";
    if (!value.startsWith("/")) {
      try { new URL(value); } catch { return setError("Enter a full URL (https://…) or a path such as /api"); }
    }
    setError("");
    onSave(value);
  };

  return (
    <div className="space-y-4 max-w-3xl">
      <div>
        <h1 className="text-[24px] font-semibold tracking-tight">Settings</h1>
        <p className="text-[13px] text-muted mt-1">Connection settings for this browser.</p>
      </div>

      <Panel title="Server status" bodyClass="p-0">
        {[
          ["API", health.api === "ok", "Connected", "Unreachable"],
          ["Database", health.db === "ok", "Online", "Offline"],
          ["Vector search", health.vector === "ok", "Online", "Unavailable"],
          ["Embeddings", health.embeddings === "enabled", "Enabled", "Keyword search only"],
        ].map(([label, ok, okText, badText]) => (
          <div key={label} className="flex items-center justify-between px-5 py-3.5 border-b border-line last:border-0">
            <span className="text-[13px]">{label}</span>
            <StatusPill ok={ok} okText={okText} badText={badText} warn={label === "Embeddings"} />
          </div>
        ))}
      </Panel>

      <Panel title="API endpoint" bodyClass="p-5">
        <form onSubmit={save} className="space-y-3">
          <Field label="Base URL" hint="Leave as /api on app.memro.co. Point it at your own server when self-hosting.">
            <input className="input font-mono" value={url} onChange={(e) => setUrl(e.target.value)} />
          </Field>
          {error && <p className="text-[12px] text-danger">{error}</p>}
          <div className="flex gap-2">
            <button className="btn-primary">Save</button>
            <button type="button" className="btn-ghost" onClick={() => { setUrl("/api"); onSave("/api"); }}>Reset to default</button>
          </div>
        </form>
      </Panel>
    </div>
  );
}
