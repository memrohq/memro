import { useMemo, useState } from "react";
import { Panel, Icon, TYPE_FILL } from "../components/ui";
import { MemoryTable } from "./Memories";

const TYPES = ["episodic", "semantic", "procedural", "profile"];
const DAYS = 30;
const ROWS = 10;

const dayKey = (d) => d.toISOString().slice(0, 10);

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

/** Thin vertical bars, like the mini charts on the reference's stat cards. */
const MiniBars = ({ values }) => {
  const max = Math.max(1, ...values);
  return (
    <div className="flex items-end gap-[3px] h-8" aria-hidden="true">
      {values.map((v, i) => (
        <span
          key={i}
          className={`w-[3px] rounded-full ${i === values.length - 1 ? "bg-accent" : "bg-accent/25"}`}
          style={{ height: `${Math.max(12, (v / max) * 100)}%` }}
        />
      ))}
    </div>
  );
};

const StatCard = ({ label, icon, tint = "text-accent bg-accent-soft", value, unit, foot, bars }) => (
  <div className="card bg-panel overflow-hidden">
    <div className="bg-surface border-b border-line rounded-xl -mx-px -mt-px px-4 pt-3.5 pb-4">
      <div className="flex items-center justify-between">
        <p className="label">{label}</p>
        {icon && <span className={`size-7 rounded-lg flex items-center justify-center ${tint}`}><Icon name={icon} className="!text-[16px]" /></span>}
      </div>
      <div className="flex items-end justify-between gap-3 mt-2">
        <p className="font-mono text-[26px] leading-none tracking-tight">
          {value ?? "—"}
          {unit && <span className="font-sans text-[12px] text-muted ml-1.5 tracking-normal">{unit}</span>}
        </p>
        {bars && <MiniBars values={bars} />}
      </div>
    </div>
    <div className="px-4 h-9 flex items-center justify-between text-[12px] text-muted">
      <Icon name="info" className="text-subtle !text-[15px]" />
      <span>{foot}</span>
    </div>
  </div>
);

/** Block-grid activity chart: one column per day, filled cells = memories. */
const ActivityGrid = ({ memories }) => {
  const [hover, setHover] = useState(null);
  const days = useMemo(() => lastDays(DAYS), []);
  const counts = useMemo(() => {
    const c = Object.fromEntries(days.map((d) => [d, 0]));
    for (const m of memories) {
      const k = m.created_at?.slice(0, 10);
      if (k in c) c[k] += 1;
    }
    return days.map((d) => c[d]);
  }, [memories, days]);
  const max = Math.max(1, ...counts);
  const total = counts.reduce((a, b) => a + b, 0);

  return (
    <div>
      <div className="flex items-baseline gap-2 mb-4">
        <span className="text-[13px] text-muted">Last 30 days</span>
        <span className="font-mono text-[22px] leading-none">{total}</span>
        <span className="text-[13px] text-muted">memories</span>
      </div>
      <div className="relative flex gap-[3px] h-[180px]" onMouseLeave={() => setHover(null)}>
        {counts.map((n, col) => {
          const filled = n === 0 ? 0 : Math.max(1, Math.round((n / max) * ROWS));
          return (
            <div
              key={days[col]}
              className="flex-1 flex flex-col-reverse gap-[3px] cursor-default"
              onMouseEnter={() => setHover(col)}
            >
              {Array.from({ length: ROWS }, (_, r) => (
                <span
                  key={r}
                  className={`flex-1 rounded-[2px] transition-colors ${
                    r < filled ? (hover === col ? "bg-accent-strong" : "bg-accent") : hover === col ? "bg-accent-line" : "bg-panel"
                  }`}
                  style={r < filled && hover !== col ? { opacity: 0.45 + 0.55 * (n / max) } : undefined}
                />
              ))}
            </div>
          );
        })}
        {hover !== null && (
          <div
            className="absolute -top-2 card px-3 py-2 shadow-sm pointer-events-none text-[12px] whitespace-nowrap"
            style={{ left: `min(calc(${(hover / DAYS) * 100}% + 8px), calc(100% - 130px))` }}
          >
            <p className="text-muted">
              {new Date(days[hover]).toLocaleDateString(undefined, { day: "numeric", month: "short" })}
            </p>
            <p className="font-mono text-[14px]">{counts[hover]} memories</p>
          </div>
        )}
      </div>
      <div className="flex justify-between mt-3 font-mono text-[11px] text-subtle">
        <span>{new Date(days[0]).toLocaleDateString(undefined, { day: "numeric", month: "short" })}</span>
        <span>Today</span>
      </div>
    </div>
  );
};

const TypeBreakdown = ({ distribution, total }) => (
  <div className="space-y-4">
    <div className="flex items-baseline gap-2">
      <span className="font-mono text-[22px] leading-none">{total ?? 0}</span>
      <span className="text-[13px] text-muted">memories by type</span>
    </div>
    {TYPES.map((t) => {
      const n = distribution?.[t] || 0;
      const pct = total ? Math.round((n / total) * 100) : 0;
      return (
        <div key={t}>
          <div className="flex justify-between text-[13px] mb-1.5">
            <span className="capitalize flex items-center gap-2"><span className={`size-2 rounded-full ${TYPE_FILL[t]}`} />{t}</span>
            <span className="font-mono text-muted">{n}</span>
          </div>
          <div className="h-2 rounded-full bg-panel overflow-hidden">
            <div className={`h-full rounded-full ${TYPE_FILL[t]}`} style={{ width: `${pct}%` }} />
          </div>
        </div>
      );
    })}
  </div>
);

export default function Overview({ email, stats, usage, memories, onOpenMemories, onAdd, onDelete }) {
  const week = useMemo(() => {
    const days = lastDays(7);
    return days.map((d) => memories.filter((m) => m.created_at?.slice(0, 10) === d).length);
  }, [memories]);
  const name = email ? email.split("@")[0] : null;
  const today = new Date().toLocaleDateString(undefined, { day: "numeric", month: "short", year: "numeric" });

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <h1 className="text-[24px] font-semibold tracking-tight">Welcome back{name ? `, ${name}` : ""}</h1>
        <div className="flex items-center gap-2">
          <span className="btn-secondary pointer-events-none">
            <Icon name="calendar_today" className="text-muted !text-[16px]" />
            {today}
          </span>
          <button className="btn-primary" onClick={onAdd}>
            <Icon name="add" className="!text-[17px]" /> Add memory
          </button>
        </div>
      </div>

      <div className="grid gap-4 grid-cols-1 sm:grid-cols-2 xl:grid-cols-4">
        <StatCard label="Total memories" icon="stacks" value={stats?.total_memories} foot={`+${stats?.memories_today ?? 0} today`} bars={week} />
        <StatCard label="This week" icon="trending_up" tint="text-procedural bg-procedural/10" value={stats?.memories_this_week} unit="new" foot={`${stats?.memories_this_month ?? 0} this month`} bars={week} />
        <StatCard label="Graph edges" icon="hub" tint="text-episodic bg-episodic/10" value={usage?.graph_edges} unit="links" foot="between memories" />
        <StatCard label="Reasoning calls" icon="psychology" tint="text-profile bg-profile/10" value={usage?.reason_calls} unit="today" foot={`${usage?.evolve_calls ?? 0} evolve runs today`} />
      </div>

      {usage?.warning && (
        <div className="card px-4 py-3 text-[13px] flex items-center gap-2">
          <Icon name="warning" className="text-warn" /> {usage.warning}
        </div>
      )}

      <div className="grid gap-4 grid-cols-1 xl:grid-cols-3">
        <Panel title="Memory activity" className="xl:col-span-2" bodyClass="p-5">
          <ActivityGrid memories={memories} />
        </Panel>
        <Panel title="Memory types" bodyClass="p-5">
          <TypeBreakdown distribution={stats?.memory_types_distribution} total={stats?.total_memories} />
        </Panel>
      </div>

      <Panel
        title="Recent memories"
        bodyClass="p-0"
        action={<button className="btn-ghost h-7 text-[12px]" onClick={onOpenMemories}>View all <Icon name="arrow_forward" className="!text-[15px]" /></button>}
      >
        <MemoryTable memories={memories.slice(0, 6)} onDelete={onDelete} compact />
      </Panel>
    </div>
  );
}
