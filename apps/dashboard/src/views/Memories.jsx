import { useMemo, useState } from "react";
import { Panel, Icon, TypePill, Empty, PageHeader, Segmented, Drawer, Skeleton, timeAgo, formatDate, MEMORY_TYPES, TYPE_INFO } from "../components/ui";

const legacyPrefix = /^\[(IMAGE ANALYSIS|DOC CHUNK)\]\s*/;
const cleanContent = (content = "") => content.replace(legacyPrefix, "");
/** One-line preview: markdown heading and list markers read as noise in a list. */
const preview = (content = "") => cleanContent(content).replace(/(^|\s)#{1,6}\s+/g, "$1").replace(/(^|\n)\s*[-*]\s+/g, "$1");

/** Where a memory came from, for its icon and caption. */
function sourceOf(m) {
  const meta = m.metadata || {};
  if (m.content?.startsWith("[IMAGE ANALYSIS]") || m.asset_url) return { icon: "image", label: "Image" };
  if (meta.source) return { icon: "description", label: meta.chunks > 1 ? `${meta.source} · part ${meta.chunk} of ${meta.chunks}` : meta.source };
  if (m.file_id || m.content?.startsWith("[DOC CHUNK]")) return { icon: "description", label: "Document" };
  return { icon: "notes", label: null };
}

const MemoryRow = ({ m, onOpen, showScore, compact }) => {
  const src = sourceOf(m);
  return (
    <li>
      <button onClick={() => onOpen(m)} className="w-full text-left flex items-start gap-3.5 px-5 py-3.5 hover:bg-panel/70 transition-colors group">
        <span className="mt-0.5 size-8 rounded-[10px] bg-panel border border-line flex items-center justify-center shrink-0">
          <Icon name={src.icon} className="!text-[17px] text-subtle group-hover:text-accent transition-colors" />
        </span>
        <span className="flex-1 min-w-0">
          <span className={`text-[13.5px] leading-relaxed text-ink ${compact ? "block truncate" : "line-clamp-2"}`}>{preview(m.content)}</span>
          <span className="flex flex-wrap items-center gap-x-2.5 gap-y-1 mt-1.5 text-[12px] text-muted">
            <TypePill type={m.memory_type} />
            {src.label && <span className="truncate max-w-[260px]">{src.label}</span>}
            {!compact && m.visibility && m.visibility !== "private" && (
              <span className="inline-flex items-center gap-1 capitalize"><Icon name="public" className="!text-[14px]" />{m.visibility}</span>
            )}
          </span>
        </span>
        <span className="flex flex-col items-end gap-1.5 shrink-0 pl-2">
          <span className="text-[12px] text-subtle whitespace-nowrap">{timeAgo(m.created_at)}</span>
          {showScore && (
            <span className="inline-flex items-center gap-2" title="Relevance">
              <span className="h-1.5 w-14 rounded-full bg-panel overflow-hidden border border-line">
                <span className="block h-full bg-accent rounded-full" style={{ width: `${Math.round((m.score || 0) * 100)}%` }} />
              </span>
              <span className="tabular text-[12px] text-muted w-8 text-right">{Math.round((m.score || 0) * 100)}%</span>
            </span>
          )}
        </span>
      </button>
    </li>
  );
};

/** Plain text with markdown headings and bullets shown as such (no HTML is injected). */
const RichText = ({ text }) => (
  <div className="text-[14.5px] leading-[1.7] space-y-3">
    {text.split(/\n{2,}/).map((block, i) => {
      const h = block.match(/^(#{1,6})\s+(.*)$/s);
      if (h) return <p key={i} className={`font-semibold tracking-[-0.01em] ${h[1].length <= 2 ? "text-[16px]" : "text-[15px]"}`}>{h[2]}</p>;
      const lines = block.split("\n");
      if (lines.every((l) => /^\s*[-*]\s+/.test(l))) {
        return <ul key={i} className="list-disc pl-5 space-y-1">{lines.map((l, j) => <li key={j}>{l.replace(/^\s*[-*]\s+/, "")}</li>)}</ul>;
      }
      return <p key={i} className="whitespace-pre-wrap">{block}</p>;
    })}
  </div>
);

/** Full content, metadata and actions for one memory. */
export function MemoryDrawer({ memory, onClose, onDelete, onCopy }) {
  const [confirm, setConfirm] = useState(false);
  if (!memory) return null;
  const m = memory;
  const src = sourceOf(m);
  const meta = m.metadata && Object.keys(m.metadata).length ? m.metadata : null;
  const Row = ({ label, children }) => (
    <div className="grid grid-cols-[120px_1fr] gap-3 py-2.5 border-b border-line last:border-0 text-[13px]">
      <span className="text-muted">{label}</span>
      <span className="min-w-0 break-words">{children}</span>
    </div>
  );
  return (
    <Drawer
      open
      title="Memory"
      onClose={() => { setConfirm(false); onClose(); }}
      footer={
        confirm ? (
          <>
            <span className="mr-auto self-center text-[13px] text-muted">Delete this memory for good?</span>
            <button className="btn-ghost" onClick={() => setConfirm(false)}>Cancel</button>
            <button className="btn-danger" onClick={() => { onDelete(m.id); onClose(); }}>Delete</button>
          </>
        ) : (
          <>
            <button className="btn-ghost mr-auto text-danger hover:text-danger" onClick={() => setConfirm(true)}><Icon name="delete" className="!text-[18px]" /> Delete</button>
            <button className="btn-secondary" onClick={() => onCopy(cleanContent(m.content), "Memory text")}><Icon name="content_copy" className="!text-[17px]" /> Copy text</button>
          </>
        )
      }
    >
      <div className="flex flex-wrap items-center gap-2 mb-4">
        <TypePill type={m.memory_type} />
        {src.label && <span className="inline-flex items-center gap-1.5 text-[12.5px] text-muted"><Icon name={src.icon} className="!text-[15px]" />{src.label}</span>}
      </div>
      {m.asset_url && (
        <a href={m.asset_url} target="_blank" rel="noopener" className="block mb-4 rounded-xl overflow-hidden border border-line bg-panel">
          <img src={m.asset_url} alt="" className="w-full max-h-64 object-contain" />
        </a>
      )}
      <RichText text={cleanContent(m.content)} />

      {m.entity_tags?.length > 0 && (
        <div className="mt-5 flex flex-wrap gap-1.5">
          {m.entity_tags.map((t) => (
            <span key={t} className="h-6 px-2 rounded-md bg-panel border border-line text-[11.5px] text-muted inline-flex items-center">{t}</span>
          ))}
        </div>
      )}

      <div className="mt-6">
        <p className="label mb-1">Details</p>
        <Row label="ID">
          <button className="font-mono text-[12px] hover:text-accent inline-flex items-center gap-1" onClick={() => onCopy(m.id, "Memory ID")}>
            {m.id} <Icon name="content_copy" className="!text-[13px]" />
          </button>
        </Row>
        <Row label="Created">{formatDate(m.created_at)}</Row>
        {m.event_date && <Row label="Happened">{formatDate(m.event_date)}</Row>}
        <Row label="Visibility"><span className="capitalize">{m.visibility || "private"}</span></Row>
        {m.confidence != null && <Row label="Confidence"><span className="tabular">{Math.round(m.confidence * 100)}%</span></Row>}
        {m.access_count > 0 && <Row label="Recalled"><span className="tabular">{m.access_count} times</span></Row>}
        {m.session_id && <Row label="Session"><span className="font-mono text-[12px]">{m.session_id}</span></Row>}
      </div>

      {meta && (
        <div className="mt-6">
          <p className="label mb-2">Metadata</p>
          <pre className="text-[12px] font-mono bg-panel border border-line rounded-xl p-3.5 overflow-x-auto scroll-thin">{JSON.stringify(meta, null, 2)}</pre>
        </div>
      )}
    </Drawer>
  );
}

/** A list of memories that opens the detail drawer on click. */
export function MemoryList({ memories, onDelete, onCopy = () => {}, compact = false, showScore = false, loading = false, empty }) {
  const [open, setOpen] = useState(null);
  if (loading) {
    return (
      <ul className="divide-y divide-line">
        {Array.from({ length: compact ? 4 : 6 }, (_, i) => (
          <li key={i} className="flex items-start gap-3.5 px-5 py-4">
            <Skeleton className="size-8 rounded-[10px]" />
            <div className="flex-1 space-y-2"><Skeleton className="h-3.5 w-4/5" /><Skeleton className="h-3 w-1/3" /></div>
          </li>
        ))}
      </ul>
    );
  }
  if (!memories.length) {
    return empty || <Empty icon="neurology" title="No memories yet">Add one with “New memory”, upload a document, or call remember() from an SDK.</Empty>;
  }
  return (
    <>
      <ul className="divide-y divide-line">
        {memories.map((m) => <MemoryRow key={m.id ?? m.memory_id} m={m} onOpen={setOpen} showScore={showScore} compact={compact} />)}
      </ul>
      <MemoryDrawer memory={open} onClose={() => setOpen(null)} onDelete={onDelete} onCopy={onCopy} />
    </>
  );
}

export default function Memories({ memories, loaded, onAdd, onDelete, onUploadFiles, uploading, onCopy }) {
  const [filter, setFilter] = useState("all");
  const [query, setQuery] = useState("");
  const [dragging, setDragging] = useState(false);
  const shown = useMemo(
    () => memories.filter((m) =>
      (filter === "all" || m.memory_type === filter) && (!query || m.content?.toLowerCase().includes(query.toLowerCase()))),
    [memories, filter, query]
  );
  const counts = useMemo(() => Object.fromEntries(MEMORY_TYPES.map((t) => [t, memories.filter((m) => m.memory_type === t).length])), [memories]);

  const onDrop = (e) => {
    e.preventDefault();
    setDragging(false);
    if (e.dataTransfer.files?.length) onUploadFiles(e.dataTransfer.files);
  };

  return (
    <div
      className="space-y-6 relative"
      onDragOver={(e) => { if (e.dataTransfer.types.includes("Files")) { e.preventDefault(); setDragging(true); } }}
      onDragLeave={(e) => { if (!e.currentTarget.contains(e.relatedTarget)) setDragging(false); }}
      onDrop={onDrop}
    >
      <PageHeader title="Memories" description="Everything your agents remember. Click a memory to see all of it.">
        <label className={`btn-secondary cursor-pointer ${uploading ? "opacity-50 pointer-events-none" : ""}`}>
          <Icon name="upload" className="!text-[18px]" /> Upload files
          <input type="file" multiple className="hidden" accept="image/*,.pdf,.docx,.pptx,.xlsx,.txt,.md,.csv,.json,.yaml,.yml,.html"
            onChange={(e) => { onUploadFiles(e.target.files); e.target.value = ""; }} disabled={!!uploading} />
        </label>
        <button className="btn-primary" onClick={onAdd}><Icon name="add" className="!text-[18px]" /> New memory</button>
      </PageHeader>

      <Panel
        bodyClass="p-0"
        title={loaded ? `${shown.length.toLocaleString()} ${shown.length === 1 ? "memory" : "memories"}` : "Memories"}
        description={filter === "all" ? "Newest first" : TYPE_INFO[filter].hint}
        action={
          <div className="relative w-60 max-w-[45vw]">
            <Icon name="search" className="absolute left-2.5 top-1/2 -translate-y-1/2 text-subtle !text-[17px]" />
            <input className="input h-8 pl-8" placeholder="Filter by text…" value={query} onChange={(e) => setQuery(e.target.value)} aria-label="Filter memories" />
          </div>
        }
      >
        <div className="px-5 py-3 border-b border-line overflow-x-auto">
          <Segmented size="sm" value={filter} onChange={setFilter}
            options={[{ id: "all", label: `All ${memories.length}` }, ...MEMORY_TYPES.map((t) => ({ id: t, label: `${TYPE_INFO[t].label} ${counts[t]}` }))]} />
        </div>
        <MemoryList
          memories={shown}
          loading={!loaded}
          onDelete={onDelete}
          onCopy={onCopy}
          empty={memories.length ? <Empty icon="filter_alt_off" title="Nothing matches">Try another type or clear the text filter.</Empty> : undefined}
        />
      </Panel>

      <p className="text-center text-[12.5px] text-subtle">
        <Icon name="upload_file" className="!text-[15px] align-[-3px] mr-1" />
        Drop images, PDFs, Word, PowerPoint, Excel or text files anywhere on this page.
      </p>

      {dragging && (
        <div className="fixed inset-0 z-40 bg-accent/10 backdrop-blur-[2px] flex items-center justify-center pointer-events-none fade-in">
          <div className="bg-elevated rounded-2xl shadow-pop px-10 py-8 text-center border-2 border-dashed border-accent">
            <Icon name="upload_file" className="!text-[36px] text-accent" />
            <p className="text-[15px] font-semibold mt-2">Drop to add to memory</p>
            <p className="text-[12.5px] text-muted mt-1">Documents are split into searchable chunks.</p>
          </div>
        </div>
      )}
    </div>
  );
}
