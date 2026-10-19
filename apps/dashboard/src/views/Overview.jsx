import { useMemo, useState } from "react";
import { Panel, Icon, PageHeader, Skeleton, MEMORY_TYPES, TYPE_INFO } from "../components/ui";
import { MemoryList } from "./Memories";

const DAYS = 30;

const dayKey = (d) => d.toISOString().slice(0, 10);
const shortDate = (k) => new Date(k + "T00:00:00Z").toLocaleDateString(undefined, { day: "numeric", month: "short", timeZone: "UTC" });

function lastDays(n) {
  const out = [];
  const today = new Date();
  for (let i = n - 1; i >= 0; i--) {
    const d = new Date(today);
    d.setDate(today.getDate() - i);
    out.push(dayKey(d));
  }
  return out;
}

function countByDay(memories, days) {
  const c = Object.fromEntries(days.map((d) => [d, 0]));
  for (const m of memories) {
    const k = m.created_at?.slice(0, 10);
    if (k in c) c[k] += 1;
  }
  return days.map((d) => c[d]);
}

/** A small single-series area line; the stat's number is the headline. */
const Sparkline = ({ values }) => {
  const w = 96, h = 32, max = Math.max(1, ...values);
  const pts = values.map((v, i) => [(i / Math.max(1, values.length - 1)) * w, h - 3 - (v / max) * (h - 6)]);
  const line = pts.map(([x, y], i) => `${i ? "L" : "M"}${x.toFixed(1)},${y.toFixed(1)}`).join(" ");
  return (
    <svg width={w} height={h} viewBox={`0 0 ${w} ${h}`} aria-hidden="true" className="overflow-visible">
      <defs>
        <linearGradient id="spark" x1="0" x2="0" y1="0" y2="1">
          <stop offset="0" stopColor="var(--accent)" stopOpacity="0.22" />
          <stop offset="1" stopColor="var(--accent)" stopOpacity="0" />
        </linearGradient>
      </defs>
      <path d={`${line} L${w},${h} L0,${h} Z`} fill="url(#spark)" />
      <path d={line} fill="none" stroke="var(--accent)" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" />
      <circle cx={pts.at(-1)[0]} cy={pts.at(-1)[1]} r="3" fill="var(--accent)" stroke="var(--surface)" strokeWidth="2" />
    </svg>
  );
};

const StatCard = ({ label, icon, value, unit, foot, spark, loading }) => (
  <div className="card p-5 flex flex-col">
    <div className="flex items-center justify-between">
      <p className="text-[13px] text-muted">{label}</p>
      <span className="size-8 rounded-[10px] bg-accent-soft border border-accent-line flex items-center justify-center">
        <Icon name={icon} className="!text-[17px] text-accent" />
      </span>
    </div>
    <div className="flex items-end justify-between gap-3 mt-3">
      {loading ? <Skeleton className="h-8 w-20" /> : (
        <p className="text-[30px] leading-none font-semibold tracking-[-0.03em] tabular">
          {value ?? 0}
          {unit && <span className="text-[13px] font-normal text-muted ml-1.5 tracking-normal">{unit}</span>}
        </p>
      )}
      {spark && !loading && <Sparkline values={spark} />}
    </div>
    <p className="text-[12.5px] text-muted mt-3">{loading ? <Skeleton className="h-3.5 w-28" /> : foot}</p>
  </div>
);

/** Memories per day: one series, rounded bars anchored to the baseline, hover tooltip. */
const ActivityChart = ({ memories }) => {
  const [hover, setHover] = useState(null);
  const days = useMemo(() => lastDays(DAYS), []);
  const counts = useMemo(() => countByDay(memories, days), [memories, days]);
  const max = Math.max(1, ...counts);
  const total = counts.reduce((a, b) => a + b, 0);
  const ticks = [max, Math.round(max / 2), 0].filter((v, i, a) => a.indexOf(v) === i);

  return (
    <div>
      <div className="flex items-baseline gap-2 mb-5">
        <span className="text-[28px] leading-none font-semibold tracking-[-0.03em] tabular">{total}</span>
        <span className="text-[13px] text-muted">memories in the last 30 days</span>
      </div>
      <div className="flex gap-3">
        <div className="relative h-[180px] w-6 shrink-0 text-[11px] text-subtle tabular" aria-hidden="true">
          {ticks.map((t) => (
            <span key={t} className="absolute right-0 -translate-y-1/2" style={{ top: `${100 - (t / max) * 100}%` }}>{t}</span>
          ))}
        </div>
        <div className="relative flex-1 h-[180px]" onMouseLeave={() => setHover(null)} role="img"
          aria-label={`Memories per day over the last 30 days, ${total} in total`}>
          {ticks.map((t) => (
            <div key={t} className="absolute inset-x-0 border-t border-dashed border-line" style={{ top: `${100 - (t / max) * 100}%` }} />
          ))}
          <div className="absolute inset-0 flex items-end gap-[3px]">
            {counts.map((n, i) => (
              <div key={days[i]} className="flex-1 h-full flex items-end cursor-default" onMouseEnter={() => setHover(i)}>
                <div
                  className={`w-full rounded-t-[4px] transition-colors ${n === 0 ? "bg-line" : hover === i ? "bg-accent-strong" : "bg-accent"}`}
                  style={{ height: n === 0 ? "2px" : `${Math.max(4, (n / max) * 100)}%`, opacity: hover === null || hover === i ? 1 : 0.55 }}
                />
              </div>
            ))}
          </div>
          {hover !== null && (
            <div className="absolute -top-3 bg-elevated rounded-[10px] shadow-pop px-3 py-2 pointer-events-none text-[12px] whitespace-nowrap z-10"
              style={{ left: `clamp(0px, calc(${((hover + 0.5) / DAYS) * 100}% - 60px), calc(100% - 120px))` }}>
              <p className="text-muted">{shortDate(days[hover])}</p>
              <p className="font-semibold text-[14px] tabular">{counts[hover]} {counts[hover] === 1 ? "memory" : "memories"}</p>
            </div>
          )}
        </div>
      </div>
      <div className="flex justify-between mt-3 pl-9 text-[11.5px] text-subtle">
        <span>{shortDate(days[0])}</span>
        <span>{shortDate(days[15])}</span>
        <span>Today</span>
      </div>
    </div>
  );
};

/** Share of memories by type: labelled bars (name, count and share), so color is never the only cue. */
const TypeBreakdown = ({ distribution, total }) => (
  <div className="space-y-4">
    <div className="flex h-2.5 rounded-full overflow-hidden bg-panel gap-[2px]" aria-hidden="true">
      {MEMORY_TYPES.map((t) => {
        const n = distribution?.[t] || 0;
        return n ? <div key={t} className={TYPE_INFO[t].fill} style={{ width: `${(n / total) * 100}%` }} /> : null;
      })}
    </div>
    <ul className="space-y-3">
      {MEMORY_TYPES.map((t) => {
        const n = distribution?.[t] || 0;
        const pct = total ? Math.round((n / total) * 100) : 0;
        return (
          <li key={t} className="flex items-center gap-3 text-[13px]">
            <span className={`size-2.5 rounded-[3px] ${TYPE_INFO[t].fill}`} />
            <span className="flex-1">
              <span className="font-medium">{TYPE_INFO[t].label}</span>
              <span className="text-muted ml-2 hidden sm:inline">{TYPE_INFO[t].hint}</span>
            </span>
            <span className="tabular font-medium">{n}</span>
            <span className="tabular text-muted w-10 text-right">{pct}%</span>
          </li>
        );
      })}
    </ul>
  </div>
);

const Step = ({ done, icon, title, children, action }) => (
  <li className="flex items-start gap-3.5 py-3.5 first:pt-0 last:pb-0">
    <span className={`size-8 rounded-full flex items-center justify-center shrink-0 border ${done ? "bg-ok/10 border-ok/30 text-ok" : "bg-panel border-line text-subtle"}`}>
      <Icon name={done ? "check" : icon} className="!text-[17px]" />
    </span>
    <div className="flex-1 min-w-0">
      <p className={`text-[13.5px] font-medium ${done ? "text-muted line-through decoration-line-strong" : ""}`}>{title}</p>
      <p className="text-[12.5px] text-muted mt-0.5">{children}</p>
    </div>
    {!done && action}
  </li>
);

export default function Overview({ email, stats, usage, memories, loaded, onOpenMemories, onAdd, onDelete, onConnect, onUpload }) {
  const week = useMemo(() => countByDay(memories, lastDays(7)), [memories]);
  const name = email ? email.split("@")[0] : null;
  const hour = new Date().getHours();
  const greeting = hour < 12 ? "Good morning" : hour < 18 ? "Good afternoon" : "Good evening";
  const total = stats?.total_memories ?? memories.length;
  const loading = !loaded;
  const fresh = loaded && total === 0;

  return (
    <div className="space-y-6">
      <PageHeader title={`${greeting}${name ? `, ${name}` : ""}`} description="Here's what your agents have been remembering.">
        <button className="btn-secondary" onClick={onUpload}><Icon name="upload" className="!text-[18px]" /> Upload</button>
        <button className="btn-primary" onClick={onAdd}><Icon name="add" className="!text-[18px]" /> New memory</button>
      </PageHeader>

      {fresh && (
        <Panel title="Get started" description="Three steps to give your AI tools a memory.">
          <ol>
            <Step icon="cable" title="Connect your coding tools" action={<button className="btn-primary h-8" onClick={onConnect}>Connect</button>}>
              Run <code className="font-mono text-[12px] px-1.5 py-0.5 rounded-md bg-panel border border-line">npx memro connect</code> or set up a tool by hand.
            </Step>
            <Step icon="edit_note" title="Add your first memory" action={<button className="btn-secondary h-8" onClick={onAdd}>Add</button>}>
              A preference, a project fact, or how your team works.
            </Step>
            <Step icon="description" title="Upload a document" action={<button className="btn-secondary h-8" onClick={onUpload}>Upload</button>}>
              PDF, Word, PowerPoint, Excel or text. Memro splits it into searchable memories.
            </Step>
          </ol>
        </Panel>
      )}

      <div className="grid gap-4 grid-cols-1 sm:grid-cols-2 xl:grid-cols-4">
        <StatCard loading={loading} label="Total memories" icon="stacks" value={total.toLocaleString?.() ?? total} spark={week}
          foot={`${stats?.memories_today ?? 0} added today`} />
        <StatCard loading={loading} label="This week" icon="trending_up" value={stats?.memories_this_week ?? 0} unit="new" spark={week}
          foot={`${stats?.memories_this_month ?? 0} this month`} />
        <StatCard loading={loading} label="Knowledge links" icon="hub" value={usage?.graph_edges ?? 0} unit="links"
          foot="Connections between memories" />
        <StatCard loading={loading} label="Reasoning today" icon="psychology" value={usage?.reason_calls ?? 0} unit="calls"
          foot={`${usage?.evolve_calls ?? 0} consolidation runs`} />
      </div>

      {usage?.warning && (
        <div className="card px-4 py-3 text-[13px] flex items-center gap-2.5 border-warn/30 bg-warn/5">
          <Icon name="warning" fill className="text-warn" /> {usage.warning}
        </div>
      )}

      <div className="grid gap-4 grid-cols-1 xl:grid-cols-3">
        <Panel title="Memory activity" description="New memories per day" className="xl:col-span-2">
          {loading ? <Skeleton className="h-[220px] w-full" /> : <ActivityChart memories={memories} />}
        </Panel>
        <Panel title="By type" description="How your memories are classified">
          {loading ? <Skeleton className="h-[220px] w-full" /> : <TypeBreakdown distribution={stats?.memory_types_distribution} total={total} />}
        </Panel>
      </div>

      <Panel
        title="Recent memories"
        bodyClass="p-0"
        action={<button className="btn-ghost h-8 text-[12.5px] -mr-2" onClick={onOpenMemories}>View all <Icon name="arrow_forward" className="!text-[16px]" /></button>}
      >
        <MemoryList memories={memories.slice(0, 6)} loading={loading} onDelete={onDelete} compact />
      </Panel>
    </div>
  );
}
