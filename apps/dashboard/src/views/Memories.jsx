import { useMemo, useState } from "react";
import { Panel, Icon, TypePill, Empty, timeAgo } from "../components/ui";

const FILTERS = ["all", "episodic", "semantic", "procedural", "profile"];

const sourceIcon = (content = "") =>
  content.startsWith("[IMAGE ANALYSIS]") ? "image" : content.startsWith("[DOC CHUNK]") ? "description" : "notes";

const cleanContent = (content = "") => content.replace(/^\[(IMAGE ANALYSIS|DOC CHUNK)\]\s*/, "");

export function MemoryTable({ memories, onDelete, compact = false, showScore = false }) {
  const [confirm, setConfirm] = useState(null);
  if (!memories.length) {
    return <Empty icon="psychology" title="No memories yet">Add one with “Add memory”, or call remember() from an SDK.</Empty>;
  }
  return (
    <div className="overflow-x-auto scroll-thin">
      <table className="w-full text-[13px]">
        <thead>
          <tr className="text-left border-b border-line">
            <th className="label font-normal py-2.5 pl-4 pr-3 w-[110px]">ID</th>
            <th className="label font-normal py-2.5 px-3">Content</th>
            <th className="label font-normal py-2.5 px-3 w-[120px]">Type</th>
            {showScore ? (
              <th className="label font-normal py-2.5 px-3 w-[140px]">Relevance</th>
            ) : (
              <th className="label font-normal py-2.5 px-3 w-[100px]">Visibility</th>
            )}
            <th className="label font-normal py-2.5 px-3 w-[110px]">Created</th>
            {onDelete && <th className="label font-normal py-2.5 pl-3 pr-4 w-[60px] text-right"> </th>}
          </tr>
        </thead>
        <tbody>
          {memories.map((m) => (
            <tr key={m.id} className="border-b border-line last:border-0 hover:bg-panel/60 group">
              <td className="py-3 pl-4 pr-3 font-mono text-[12px] text-muted whitespace-nowrap">
                <span className="inline-flex items-center gap-1.5">
                  <Icon name={sourceIcon(m.content)} className="!text-[15px] text-subtle" />
                  {String(m.id).slice(0, 8)}
                </span>
              </td>
              <td className="py-3 px-3">
                <p className={compact ? "truncate max-w-[520px]" : "line-clamp-2"}>{cleanContent(m.content)}</p>
              </td>
              <td className="py-3 px-3"><TypePill type={m.memory_type} /></td>
              {showScore ? (
                <td className="py-3 px-3">
                  <div className="flex items-center gap-2">
                    <div className="h-1.5 w-16 rounded-full bg-panel overflow-hidden">
                      <div className="h-full bg-accent rounded-full" style={{ width: `${Math.round((m.score || 0) * 100)}%` }} />
                    </div>
                    <span className="font-mono text-[12px] text-muted">{Math.round((m.score || 0) * 100)}%</span>
                  </div>
                </td>
              ) : (
                <td className="py-3 px-3 text-muted capitalize">{m.visibility || "private"}</td>
              )}
              <td className="py-3 px-3 text-muted whitespace-nowrap">{timeAgo(m.created_at)}</td>
              {onDelete && (
                <td className="py-3 pl-3 pr-4 text-right">
                  {confirm === m.id ? (
                    <span className="inline-flex gap-1">
                      <button className="btn-danger h-7 px-2 text-[12px]" onClick={() => { setConfirm(null); onDelete(m.id); }}>Delete</button>
                      <button className="btn-ghost h-7 px-2 text-[12px]" onClick={() => setConfirm(null)}>Cancel</button>
                    </span>
                  ) : (
                    <button
                      className="btn-ghost size-7 px-0 opacity-0 group-hover:opacity-100 focus:opacity-100"
                      aria-label="Delete memory"
                      onClick={() => setConfirm(m.id)}
                    >
                      <Icon name="delete" className="!text-[17px]" />
                    </button>
                  )}
                </td>
              )}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

export default function Memories({ memories, onAdd, onDelete, onUpload, uploading }) {
  const [filter, setFilter] = useState("all");
  const [query, setQuery] = useState("");
  const shown = useMemo(
    () =>
      memories.filter(
        (m) =>
          (filter === "all" || m.memory_type === filter) &&
          (!query || m.content?.toLowerCase().includes(query.toLowerCase()))
      ),
    [memories, filter, query]
  );

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 className="text-[24px] font-semibold tracking-tight">Memories</h1>
          <p className="text-[13px] text-muted mt-1">Everything your agent remembers, newest first.</p>
        </div>
        <div className="flex items-center gap-2">
          <label className={`btn-secondary cursor-pointer ${uploading ? "opacity-50 pointer-events-none" : ""}`}>
            <Icon name="upload" className="!text-[17px]" /> {uploading ? "Processing…" : "Upload image or PDF"}
            <input type="file" accept="image/*,application/pdf" className="hidden" onChange={onUpload} disabled={uploading} />
          </label>
          <button className="btn-primary" onClick={onAdd}><Icon name="add" className="!text-[17px]" /> Add memory</button>
        </div>
      </div>

      <Panel
        title={`${shown.length} of ${memories.length} memories`}
        bodyClass="p-0"
        action={
          <div className="relative w-56 max-w-[45vw]">
            <Icon name="search" className="absolute left-2.5 top-1/2 -translate-y-1/2 text-subtle !text-[16px]" />
            <input className="input h-8 pl-8" placeholder="Filter this list…" value={query} onChange={(e) => setQuery(e.target.value)} />
          </div>
        }
      >
        <div className="flex gap-1 px-3 pt-3 pb-2 border-b border-line overflow-x-auto">
          {FILTERS.map((f) => (
            <button
              key={f}
              onClick={() => setFilter(f)}
              className={`h-7 px-3 rounded-md text-[12px] capitalize transition-colors ${
                filter === f ? "bg-panel border border-line text-ink" : "text-muted hover:text-ink"
              }`}
            >
              {f}
            </button>
          ))}
        </div>
        <MemoryTable memories={shown} onDelete={onDelete} />
      </Panel>
    </div>
  );
}
