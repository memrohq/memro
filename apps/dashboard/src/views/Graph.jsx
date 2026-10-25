import { Suspense, lazy, useCallback, useEffect, useMemo, useRef, useState } from "react";
import * as THREE from "three";
import { Icon, PageHeader, Empty, TypePill, TYPE_INFO, formatDate } from "../components/ui";
import { api } from "../lib/api";

// The page itself is lazy-loaded (App.jsx), so three.js stays out of the main bundle.
const ForceGraph3D = lazy(() => import("react-force-graph-3d"));

const GRAVITY_ID = "__gravity__";
const MAX_NODES = 400; // above this the layout gets slow; keep the biggest nodes

/** Link styles, keyed by link type from GET /memory/graph. */
const LINK_INFO = {
  updates: { label: "Replaces", color: "--danger" },
  extends: { label: "Extends", color: "--accent" },
  derives: { label: "Derived from", color: "--accent" },
  associated: { label: "Related", color: "--accent" },
  causes: { label: "Causes", color: "--warn" },
  mentions: { label: "Mentions", color: "--subtle" },
  same_session: { label: "Same conversation", color: "--line-strong" },
};

const cssVar = (name) => getComputedStyle(document.documentElement).getPropertyValue(name).trim() || "#888888";
const endId = (end) => (typeof end === "object" ? end.id : end);

/** Theme colors, re-read when light/dark changes. */
function usePalette(dark) {
  return useMemo(() => ({
    type: Object.fromEntries(Object.keys(TYPE_INFO).map((t) => [t, cssVar(`--t-${t}`)])),
    entity: cssVar("--muted"),
    ink: cssVar("--ink"),
    link: Object.fromEntries(Object.entries(LINK_INFO).map(([k, v]) => [k, cssVar(v.color)])),
  }), [dark]); // eslint-disable-line react-hooks/exhaustive-deps
}

// A soft glowing dot per color, shared by every node of that color.
const spriteCache = new Map();
function glowMaterial(color, intensity) {
  const key = `${color}_${intensity}`;
  if (spriteCache.has(key)) return spriteCache.get(key);
  const res = 256, c = res / 2, r = res / 2 - 16;
  const canvas = document.createElement("canvas");
  canvas.width = canvas.height = res;
  const ctx = canvas.getContext("2d");
  const g = ctx.createRadialGradient(c, c, 0, c, c, r);
  g.addColorStop(0, "#ffffff");
  g.addColorStop(Math.min(0.15 * intensity, 0.4), color);
  g.addColorStop(1, `${color}00`);
  ctx.fillStyle = g;
  ctx.beginPath(); ctx.arc(c, c, r, 0, 2 * Math.PI); ctx.fill();
  ctx.strokeStyle = color; ctx.globalAlpha = Math.min(0.7 * intensity, 1); ctx.lineWidth = 6;
  ctx.beginPath(); ctx.arc(c, c, r * 0.92, 0, 2 * Math.PI); ctx.stroke();
  const tex = new THREE.CanvasTexture(canvas);
  tex.minFilter = THREE.LinearFilter;
  const mat = new THREE.SpriteMaterial({ map: tex, transparent: true, depthWrite: false });
  spriteCache.set(key, mat);
  return mat;
}

function textSprite(label, color) {
  const canvas = document.createElement("canvas");
  canvas.width = 512; canvas.height = 96;
  const ctx = canvas.getContext("2d");
  ctx.font = '600 40px "Geist", ui-sans-serif, system-ui, sans-serif';
  ctx.fillStyle = color; ctx.textBaseline = "middle";
  ctx.fillText(label.length > 22 ? `${label.slice(0, 21)}…` : label, 8, 48);
  const tex = new THREE.CanvasTexture(canvas);
  tex.minFilter = THREE.LinearFilter;
  const sprite = new THREE.Sprite(new THREE.SpriteMaterial({ map: tex, transparent: true, depthWrite: false }));
  sprite.scale.set(40, 7.5, 1);
  return sprite;
}

const ToolButton = ({ icon, label, onClick, active }) => (
  <button onClick={onClick} title={label} aria-label={label} aria-pressed={active}
    className={`size-9 rounded-[10px] border flex items-center justify-center transition-colors ${active ? "bg-accent text-on-accent border-accent" : "bg-surface/90 text-muted border-line hover:text-ink hover:border-line-strong"}`}>
    <Icon name={icon} className="!text-[18px]" />
  </button>
);

const Toggle = ({ on, onChange, children }) => (
  <label className="inline-flex items-center gap-2 text-[12.5px] text-muted cursor-pointer select-none">
    <input type="checkbox" checked={on} onChange={(e) => onChange(e.target.checked)} className="accent-[var(--accent)]" />
    {children}
  </label>
);

export default function Graph({ apiBase, auth, dark, onOpenMemories }) {
  const [data, setData] = useState(null);
  const [error, setError] = useState(null);
  const [showEntities, setShowEntities] = useState(true);
  const [showSessions, setShowSessions] = useState(true);
  const reducedMotion = useMemo(() => window.matchMedia("(prefers-reduced-motion: reduce)").matches, []);
  const [rotating, setRotating] = useState(!reducedMotion);
  const [selected, setSelected] = useState(null);
  const [dims, setDims] = useState({ w: 800, h: 600 });
  const fgRef = useRef(null);
  const boxRef = useRef(null);
  const rotRef = useRef(rotating);
  const busyRef = useRef(false); // camera is moving on its own; don't rotate
  const palette = usePalette(dark);

  const load = useCallback(async () => {
    setError(null);
    try {
      setData(await api(apiBase, "/memory/graph?limit=300", { auth }));
    } catch (err) {
      setError(err.message || "Couldn't load the graph");
    }
  }, [apiBase, auth]);
  useEffect(() => { load(); }, [load]);

  useEffect(() => {
    if (!boxRef.current) return;
    const ro = new ResizeObserver(([e]) => setDims({ w: e.contentRect.width, h: e.contentRect.height }));
    ro.observe(boxRef.current);
    return () => ro.disconnect();
  }, [data]);

  // Slow orbit around the center at the current zoom, paused while the user
  // inspects a node.
  useEffect(() => { rotRef.current = rotating; }, [rotating]);
  useEffect(() => {
    let frame;
    const tick = () => {
      const fg = fgRef.current;
      if (rotRef.current && !busyRef.current && fg) {
        const { x, y, z } = fg.camera().position;
        const angle = Math.atan2(x, z) + Math.PI / 1800;
        const r = Math.hypot(x, z);
        fg.cameraPosition({ x: r * Math.sin(angle), y, z: r * Math.cos(angle) });
      }
      frame = requestAnimationFrame(tick);
    };
    const start = setTimeout(() => { frame = requestAnimationFrame(tick); }, 2500);
    return () => { clearTimeout(start); cancelAnimationFrame(frame); };
  }, []);

  const graph = useMemo(() => {
    if (!data) return { nodes: [], links: [] };
    let nodes = data.nodes.filter((n) => showEntities || n.kind !== "entity");
    if (nodes.length > MAX_NODES) nodes = [...nodes].sort((a, b) => b.val - a.val).slice(0, MAX_NODES);
    const keep = new Set(nodes.map((n) => n.id));
    const links = data.links.filter((l) => keep.has(endId(l.source)) && keep.has(endId(l.target))
      && (showSessions || l.type !== "same_session"));
    // An invisible center every node is tied to keeps islands in one cluster.
    return {
      nodes: [...nodes.map((n) => ({ ...n })), { id: GRAVITY_ID, gravity: true, val: 0 }],
      links: [...links.map((l) => ({ ...l })), ...nodes.map((n) => ({ source: n.id, target: GRAVITY_ID, gravity: true }))],
    };
  }, [data, showEntities, showSessions]);

  const degree = useMemo(() => {
    const d = {};
    for (const l of data?.links || []) {
      d[endId(l.source)] = (d[endId(l.source)] || 0) + 1;
      d[endId(l.target)] = (d[endId(l.target)] || 0) + 1;
    }
    return d;
  }, [data]);

  const nodeColor = useCallback((n) => (n.kind === "entity" ? palette.entity : palette.type[n.memory_type] || palette.entity), [palette]);

  const nodeObject = useCallback((n) => {
    if (n.gravity) return new THREE.Group();
    const isEntity = n.kind === "entity";
    const size = Math.sqrt(n.val || 1) * (isEntity ? 5 : 4) + 2;
    const sprite = new THREE.Sprite(glowMaterial(nodeColor(n), isEntity ? 1.4 : n.memory_type === "profile" ? 1.2 : 0.8));
    sprite.scale.set(size, size, 1);
    if (!isEntity || (degree[n.id] || 0) < 3) return sprite;
    const group = new THREE.Group();
    group.add(sprite);
    const label = textSprite(n.label, palette.ink);
    label.position.set(size * 0.5 + 21, size * 0.4, 0);
    group.add(label);
    return group;
  }, [nodeColor, degree, palette]);

  const linkColor = useCallback((l) => (l.gravity ? "rgba(0,0,0,0)" : palette.link[l.type] || palette.entity), [palette]);

  const forces = useCallback(() => {
    const fg = fgRef.current;
    if (!fg) return;
    fg.d3Force("charge")?.strength(-6);
    fg.d3Force("link")?.distance((l) => (l.gravity ? 30 : l.type === "same_session" ? 6 : 14))
      .strength((l) => (l.gravity ? 0.05 : 0.6));
  }, []);
  useEffect(forces, [graph, forces]);

  const fit = useCallback(() => {
    busyRef.current = true;
    fgRef.current?.zoomToFit(700, 60, (n) => !n.gravity);
    setTimeout(() => { busyRef.current = false; }, 800);
  }, []);

  // Frame the graph soon after it loads, then again when the layout settles.
  useEffect(() => {
    if (!data) return;
    const t = setTimeout(fit, 1200);
    return () => clearTimeout(t);
  }, [graph, data, fit]);

  const focus = useCallback((n) => {
    if (n.gravity) return;
    setRotating(false);
    setSelected(n);
    busyRef.current = true;
    const r = 1 + 70 / Math.hypot(n.x || 1, n.y || 1, n.z || 1);
    fgRef.current?.cameraPosition({ x: n.x * r, y: n.y * r, z: n.z * r }, n, 900);
    setTimeout(() => { busyRef.current = false; }, 1000);
  }, []);

  const neighbours = useMemo(() => {
    if (!selected || !data) return [];
    const byId = Object.fromEntries(data.nodes.map((n) => [n.id, n]));
    return data.links
      .filter((l) => endId(l.source) === selected.id || endId(l.target) === selected.id)
      .map((l) => ({ type: l.type, node: byId[endId(l.source) === selected.id ? endId(l.target) : endId(l.source)] }))
      .filter((x) => x.node)
      .slice(0, 8);
  }, [selected, data]);

  const memoryCount = data?.nodes.filter((n) => n.kind === "memory").length ?? 0;
  const entityCount = data?.nodes.filter((n) => n.kind === "entity").length ?? 0;
  const linkTypes = useMemo(() => [...new Set((data?.links || []).map((l) => l.type))], [data]);

  return (
    <div className="space-y-5">
      <PageHeader title="Graph"
        description="Your memories in 3D. Lines join memories that replace or build on each other, mention the same thing, or come from the same conversation.">
        <Toggle on={showEntities} onChange={setShowEntities}>Entities</Toggle>
        <Toggle on={showSessions} onChange={setShowSessions}>Conversations</Toggle>
      </PageHeader>

      {error ? (
        <div className="card">
          <Empty icon="error" title="Couldn't load the graph" action={<button onClick={load} className="btn-secondary">Try again</button>}>{error}</Empty>
        </div>
      ) : data && memoryCount === 0 ? (
        <div className="card">
          <Empty icon="bubble_chart" title="No memories yet" action={onOpenMemories && <button onClick={onOpenMemories} className="btn-secondary">Go to memories</button>}>
            Memories you save show up here as a connected graph.
          </Empty>
        </div>
      ) : (
        <div className="card relative overflow-hidden" style={{ height: "calc(100vh - 230px)", minHeight: 420 }}>
          <div className="absolute top-4 left-5 z-10 pointer-events-none text-[12px] text-muted font-mono">
            {data ? `${memoryCount} memories · ${entityCount} entities · ${data.links.length} links${data.truncated ? " · newest 300" : ""}` : "Loading…"}
          </div>
          <div className="absolute top-3 right-3 z-10 flex gap-2">
            <ToolButton icon={rotating ? "pause" : "3d_rotation"} label={rotating ? "Stop rotating" : "Rotate"} active={rotating} onClick={() => setRotating((r) => !r)} />
            <ToolButton icon="center_focus_strong" label="Fit to view" onClick={fit} />
            <ToolButton icon="refresh" label="Reload" onClick={load} />
          </div>

          <div ref={boxRef} className="absolute inset-0">
            {data && (
              <Suspense fallback={<p className="p-5 pt-12 text-[13px] text-muted">Loading 3D view…</p>}>
                <ForceGraph3D
                  ref={fgRef}
                  width={dims.w}
                  height={dims.h}
                  graphData={graph}
                  backgroundColor="rgba(0,0,0,0)"
                  showNavInfo={false}
                  nodeThreeObject={nodeObject}
                  nodeLabel={(n) => (n.gravity ? "" : n.label)}
                  linkColor={linkColor}
                  linkOpacity={0.55}
                  linkWidth={(l) => (l.type === "updates" || l.type === "causes" ? 0.9 : 0.35)}
                  linkDirectionalArrowLength={(l) => (l.type === "updates" || l.type === "causes" ? 2.5 : 0)}
                  linkDirectionalArrowRelPos={1}
                  onNodeClick={focus}
                  onBackgroundClick={() => setSelected(null)}
                  onEngineStart={forces}
                  warmupTicks={80}
                  cooldownTicks={300}
                  onEngineStop={fit}
                />
              </Suspense>
            )}
          </div>

          <div className="absolute bottom-3 left-4 z-10 flex flex-wrap gap-x-4 gap-y-1.5 max-w-[60%] text-[11.5px] text-muted pointer-events-none">
            {Object.keys(TYPE_INFO).map((t) => (
              <span key={t} className="inline-flex items-center gap-1.5">
                <span className="size-2 rounded-full" style={{ background: palette.type[t] }} />{TYPE_INFO[t].label}
              </span>
            ))}
            {showEntities && entityCount > 0 && (
              <span className="inline-flex items-center gap-1.5"><span className="size-2 rounded-full" style={{ background: palette.entity }} />Entity</span>
            )}
            {linkTypes.filter((t) => LINK_INFO[t] && (showSessions || t !== "same_session") && (showEntities || t !== "mentions")).map((t) => (
              <span key={t} className="inline-flex items-center gap-1.5">
                <span className="h-0.5 w-3.5 rounded-full" style={{ background: palette.link[t] }} />{LINK_INFO[t].label}
              </span>
            ))}
          </div>

          {selected && (
            <aside aria-label="Selected node" className="absolute bottom-3 right-3 z-10 w-80 max-w-[calc(100%-24px)] card p-4 shadow-[var(--shadow-pop)]">
              <div className="flex items-start justify-between gap-3">
                <div className="min-w-0">
                  {selected.kind === "entity"
                    ? <span className="text-[12px] text-muted">Entity · mentioned by {degree[selected.id] || 0} memories</span>
                    : <span className="flex items-center gap-2 text-[12px] text-muted"><TypePill type={selected.memory_type} />{selected.at && formatDate(selected.at)}</span>}
                </div>
                <button onClick={() => setSelected(null)} aria-label="Close" className="text-subtle hover:text-ink"><Icon name="close" className="!text-[18px]" /></button>
              </div>
              <p className="mt-2 text-[13.5px] leading-relaxed text-ink line-clamp-6">{selected.content || selected.label}</p>
              {neighbours.length > 0 && (
                <ul className="mt-3 pt-3 border-t border-line space-y-1.5">
                  {neighbours.map(({ type, node }) => (
                    <li key={`${type}-${node.id}`}>
                      <button onClick={() => focus(node)} className="w-full text-left text-[12.5px] flex gap-2 hover:text-accent">
                        <span className="text-subtle shrink-0 w-[124px] truncate">{LINK_INFO[type]?.label || type}</span>
                        <span className="truncate text-ink">{node.label}</span>
                      </button>
                    </li>
                  ))}
                </ul>
              )}
            </aside>
          )}
        </div>
      )}
    </div>
  );
}
