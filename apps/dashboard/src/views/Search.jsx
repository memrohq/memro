import { useEffect, useState } from "react";
import { Panel, Icon, Empty, PageHeader, Segmented } from "../components/ui";
import { MemoryList } from "./Memories";

const MODES = [
  { id: "hybrid", label: "Hybrid" },
  { id: "semantic", label: "Semantic" },
  { id: "keyword", label: "Keyword" },
];

export default function Search({ initialQuery = "", onSearch, onDelete, onCopy }) {
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

  const SUGGESTIONS = ["What does the user prefer?", "How do we deploy?", "Who owns billing?"];

  return (
    <div className="space-y-6">
      <PageHeader title="Search" description="Recall memories the way your agents do, before they prompt the model." />

      <form onSubmit={run} className="card p-2 flex flex-wrap items-center gap-2 focus-within:border-accent focus-within:ring-4 focus-within:ring-accent/10 transition-[border-color,box-shadow]">
        <div className="relative flex-1 min-w-[220px]">
          <Icon name="search" className="absolute left-3 top-1/2 -translate-y-1/2 text-subtle" />
          <input
            autoFocus
            className="w-full h-11 pl-10 pr-3 bg-transparent outline-none text-[15px] placeholder:text-subtle"
            placeholder="Ask anything your agents might know…"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            aria-label="Search query"
          />
        </div>
        <Segmented options={MODES} value={mode} onChange={setMode} />
        <button className="btn-primary h-10 px-5" disabled={busy || !query.trim()}>
          {busy ? <span className="size-4 rounded-full border-2 border-on-accent/30 border-t-on-accent animate-spin" /> : <Icon name="arrow_forward" className="!text-[18px]" />}
          Search
        </button>
      </form>

      {error && <div className="card px-4 py-3 text-[13px] text-danger flex items-center gap-2"><Icon name="error" fill /> {error}</div>}

      {result ? (
        <Panel
          title={`${result.results.length} ${result.results.length === 1 ? "result" : "results"}`}
          description={`“${query.trim()}”`}
          bodyClass="p-0"
          action={<span className="h-6 px-2 rounded-md bg-panel border border-line text-[11.5px] text-muted inline-flex items-center gap-1.5 capitalize tabular">{result.mode} · {result.latency_ms} ms</span>}
        >
          <MemoryList
            showScore
            onDelete={onDelete}
            onCopy={onCopy}
            empty={<Empty icon="search_off" title="No matches">Try other words, or switch to {mode === "keyword" ? "hybrid" : "keyword"} search.</Empty>}
            memories={result.results.map((r) => ({
              id: r.memory_id, content: r.content, memory_type: r.memory_type, score: r.score,
              created_at: r.created_at, asset_url: r.asset_url,
            }))}
          />
        </Panel>
      ) : (
        <div className="card">
          <Empty icon="manage_search" title="Search your memory"
            action={
              <div className="flex flex-wrap justify-center gap-2">
                {SUGGESTIONS.map((q) => (
                  <button key={q} className="btn-secondary h-8 text-[12.5px]" onClick={() => { setQuery(q); }}>{q}</button>
                ))}
              </div>
            }>
            Hybrid combines meaning and keywords. Semantic finds paraphrases; keyword finds exact names and IDs.
          </Empty>
        </div>
      )}
    </div>
  );
}
