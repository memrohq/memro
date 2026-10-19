import { useEffect, useState } from "react";
import { Panel, Icon, Empty } from "../components/ui";
import { MemoryTable } from "./Memories";

const MODES = [
  { id: "hybrid", label: "Hybrid" },
  { id: "semantic", label: "Semantic" },
  { id: "keyword", label: "Keyword" },
];

export default function Search({ initialQuery = "", onSearch }) {
  const [query, setQuery] = useState(initialQuery);
  const [mode, setMode] = useState("hybrid");
  const [result, setResult] = useState(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  const run = async (e) => {
    e?.preventDefault();
    if (!query.trim()) return;
    setBusy(true);
    setError("");
    try {
      setResult(await onSearch(query.trim(), mode));
    } catch (err) {
      setError(err.message);
    } finally {
      setBusy(false);
    }
  };

  // A query from the header search box runs straight away.
  useEffect(() => {
    if (initialQuery) run();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  return (
    <div className="space-y-4">
      <div>
        <h1 className="text-[24px] font-semibold tracking-tight">Search</h1>
        <p className="text-[13px] text-muted mt-1">Recall memories the way your agent does, before it prompts the model.</p>
      </div>

      <form onSubmit={run} className="card p-2 flex flex-wrap items-center gap-2">
        <div className="relative flex-1 min-w-[220px]">
          <Icon name="search" className="absolute left-3 top-1/2 -translate-y-1/2 text-subtle" />
          <input
            autoFocus
            className="input h-10 pl-9 border-transparent focus:border-line"
            placeholder="What does the user prefer for meetings?"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
          />
        </div>
        <div className="flex rounded-lg bg-panel border border-line p-0.5">
          {MODES.map((m) => (
            <button
              key={m.id}
              type="button"
              onClick={() => setMode(m.id)}
              className={`h-8 px-3 rounded-md text-[12px] transition-colors ${mode === m.id ? "bg-surface border border-line text-ink" : "text-muted hover:text-ink"}`}
            >
              {m.label}
            </button>
          ))}
        </div>
        <button className="btn-primary h-10" disabled={busy || !query.trim()}>{busy ? "Searching…" : "Search"}</button>
      </form>

      {error && <div className="card px-4 py-3 text-[13px] text-danger">{error}</div>}

      {result ? (
        <Panel
          title={`${result.results.length} results`}
          bodyClass="p-0"
          action={<span className="font-mono text-[11px] text-subtle">{result.mode} · {result.latency_ms} ms</span>}
        >
          <MemoryTable
            showScore
            memories={result.results.map((r) => ({
              id: r.memory_id,
              content: r.content,
              memory_type: r.memory_type,
              score: r.score,
              created_at: r.created_at,
            }))}
          />
        </Panel>
      ) : (
        <div className="card">
          <Empty icon="manage_search" title="Search your agent's memory">
            Hybrid search combines meaning and keywords. Semantic search needs an embedding key on the server.
          </Empty>
        </div>
      )}
    </div>
  );
}
