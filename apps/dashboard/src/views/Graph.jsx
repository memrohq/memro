import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import GraphModel from "graphology";
import FA2Layout from "graphology-layout-forceatlas2/worker";
import forceAtlas2 from "graphology-layout-forceatlas2";
import louvain from "graphology-communities-louvain";
import Sigma from "sigma";
import { Icon, PageHeader, Empty, TypePill, TYPE_INFO, formatDate } from "../components/ui";
import { api } from "../lib/api";

// The memory graph, drawn with WebGL (sigma.js) in 2D: thousands of nodes stay
// smooth, people and companies are labelled, hovering lights up a node's
// neighbourhood. A view is capped (SIZES) by design; the summary view for
// millions of memories builds on this (see docs/decisions).

const SIZES = [300, 1000, 5000]; // 5,000 is the server's maximum

// Cluster colours: the dataviz reference palette's first three slots, the only
// ones that stay distinguishable in every pairing (colour-blind and normal
// vision) when dots sit side by side; every other cluster is neutral. Each
// coloured cluster is also labelled, so colour is never the only cue.
const CLUSTER_COLORS = { light: ["#2a78d6", "#eb6834", "#1baf7a"], dark: ["#3987e5", "#d95926", "#199e70"] };

// Tighter, more separated clusters than ForceAtlas2's defaults (LinLog pulls
// communities together; hubs are pushed outward).
const layoutSettings = (g) => ({
  ...forceAtlas2.inferSettings(g), linLogMode: true, outboundAttractionDistribution: true,
  gravity: 1, scalingRatio: 10, slowDown: 2, barnesHutOptimize: g.order > 500,
});

/** Link styles, keyed by link type from GET /memory/graph. */
const LINK_INFO = {
  updates: { label: "Replaces", color: "--danger" },
  extends: { label: "Extends", color: "--accent" },
  derives: { label: "Derived from", color: "--accent" },
  associated: { label: "Related", color: "--accent" },
  causes: { label: "Causes", color: "--warn" },
  mentions: { label: "Mentions", color: "--subtle" },
  same_session: { label: "Same conversation", color: "--muted" },
};

// Browsers may shorten a token to #rgb; colours here are mixed as #rrggbb.
const cssVar = (name) => {
  const v = getComputedStyle(document.documentElement).getPropertyValue(name).trim() || "#888888";
  return /^#[0-9a-f]{3}$/i.test(v) ? `#${[...v.slice(1)].map((c) => c + c).join("")}` : v;
};
const endId = (end) => (typeof end === "object" ? end.id : end);
// #rrggbb as rgba() with an alpha, for faint links and dimmed nodes (WebGL
// doesn't take 8-digit hex).
const alpha = (hex, a) => {
  const m = /^#([0-9a-f]{2})([0-9a-f]{2})([0-9a-f]{2})$/i.exec(hex);
  return m ? `rgba(${parseInt(m[1], 16)},${parseInt(m[2], 16)},${parseInt(m[3], 16)},${a})` : hex;
};

// Solid colour part way from a to b; WebGL washes out transparent nodes, so
// dimmed nodes are mixed towards the background instead.
const mix = (a, b, t) => {
  const p = (h) => [1, 3, 5].map((i) => parseInt(h.slice(i, i + 2), 16));
  if (!/^#[0-9a-f]{6}$/i.test(a) || !/^#[0-9a-f]{6}$/i.test(b)) return a;
  const [x, y] = [p(a), p(b)];
  return `#${x.map((v, i) => Math.round(v + (y[i] - v) * t).toString(16).padStart(2, "0")).join("")}`;
};

// The label box shown on hover, in the theme's colours (sigma's default is
// white, unreadable in dark mode).
const hoverDrawer = (pal) => (ctx, data, settings) => {
  const size = settings.labelSize, font = settings.labelFont, weight = settings.labelWeight;
  ctx.font = `${weight} ${size}px ${font}`;
  const label = data.label || "";
  const w = label ? ctx.measureText(label).width + 12 : 0, h = size + 10;
  ctx.fillStyle = pal.surface;
  ctx.strokeStyle = pal.line;
  ctx.lineWidth = 1;
  ctx.beginPath();
  ctx.arc(data.x, data.y, data.size + 3, 0, Math.PI * 2);
  ctx.fill(); ctx.stroke();
  if (label) {
    const x = data.x + data.size + 6, y = data.y - h / 2;
    ctx.beginPath();
    ctx.roundRect(x, y, w, h, 6);
    ctx.fill(); ctx.stroke();
    ctx.fillStyle = pal.ink;
    ctx.fillText(label, x + 6, data.y + size / 3);
  }
};

// Two looks (ADR 0031): Studio Mono in light (neutral at rest, the accent only
// where you look) and Observatory in dark (deep background, glowing names,
// faint hazes behind clusters, drawn by the glow layer below).
const readPalette = () => {
  const dark = document.documentElement.classList.contains("dark");
  const muted = cssVar("--muted"), surface = cssVar("--surface"), ink = cssVar("--ink"), accent = cssVar("--accent");
  return {
    dark,
    clusters: CLUSTER_COLORS[dark ? "dark" : "light"],
    highlight: dark ? "#ffffff" : accent,
    glow: accent,
    bg: dark ? "#06070c" : surface,
    calm: dark
      ? { memory: "#7d88b8", entity: "#e8ecff", edge: mix("#06070c", "#96aaff", 0.16) }
      : { memory: mix(muted, surface, 0.68), entity: mix(ink, surface, 0.22), edge: mix(surface, ink, 0.1) },
    ...readTokens(),
  };
};
const readTokens = () => ({
  type: Object.fromEntries(Object.keys(TYPE_INFO).map((t) => [t, cssVar(`--t-${t}`)])),
  entity: cssVar("--muted"),
  ink: cssVar("--ink"),
  muted: cssVar("--muted"),
  surface: cssVar("--surface"),
  line: cssVar("--line-strong"),
  link: Object.fromEntries(Object.entries(LINK_INFO).map(([k, v]) => [k, cssVar(v.color)])),
});

// Observatory's glow, on a 2D canvas under sigma's WebGL layers: a faint haze
// behind each coloured cluster and a soft halo on the labelled names and the
// focus. Dark mode only; drawn after every frame, so it follows pan and zoom.
function drawGlow(renderer, canvas, model, palette, focus) {
  const { width, height } = renderer.getDimensions();
  const dpr = window.devicePixelRatio || 1;
  if (canvas.width !== width * dpr) { canvas.width = width * dpr; canvas.height = height * dpr; }
  const ctx = canvas.getContext("2d");
  ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
  ctx.clearRect(0, 0, width, height);
  if (!palette.dark) return;
  const halo = (x, y, r, color, a) => {
    const g = ctx.createRadialGradient(x, y, 0, x, y, r);
    g.addColorStop(0, alpha(color, a));
    g.addColorStop(1, alpha(color, 0));
    ctx.fillStyle = g;
    ctx.fillRect(x - r, y - r, r * 2, r * 2);
  };
  for (const c of model.getAttribute("clusters") || []) {
    let sx = 0, sy = 0, n = 0;
    const pts = [];
    model.forEachNode((id, a) => {
      if (a.community !== c.community) return;
      const p = renderer.graphToViewport(a);
      pts.push(p); sx += p.x; sy += p.y; n++;
    });
    if (!n) continue;
    const cx = sx / n, cy = sy / n;
    const spread = Math.sqrt(pts.reduce((t, p) => t + (p.x - cx) ** 2 + (p.y - cy) ** 2, 0) / n);
    halo(cx, cy, Math.max(60, spread * 1.6), c.color, 0.1);
  }
  model.forEachNode((id, a) => {
    if (!(a.forceLabel || id === focus)) return;
    const p = renderer.graphToViewport(a);
    const r = renderer.scaleSize(a.size) * (id === focus ? 9 : 5);
    halo(p.x, p.y, r, id === focus ? palette.glow : "#9fb4ff", id === focus ? 0.55 : 0.22);
  });
}

/** Theme colors, re-read whenever the page's theme class changes (the app
 * switches it after this view has rendered, so a prop alone is too early). */
function usePalette() {
  const [palette, setPalette] = useState(readPalette);
  useEffect(() => {
    const mo = new MutationObserver(() => setPalette(readPalette()));
    mo.observe(document.documentElement, { attributes: true, attributeFilter: ["class", "data-theme"] });
    setPalette(readPalette());
    return () => mo.disconnect();
  }, []);
  return palette;
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

// A node's neighbours (near) and, for depth 2, theirs (far): its blast radius.
// The second ring is capped (300) so a hub doesn't light up the whole graph.
function rings(g, focus, depth) {
  const near = new Set(g.neighbors(focus));
  const far = new Set();
  if (depth > 1) {
    for (const n of near) {
      for (const m of g.neighbors(n)) {
        if (m !== focus && !near.has(m)) far.add(m);
        if (far.size >= 300) break;
      }
      if (far.size >= 300) break;
    }
  }
  return { focus, near, far };
}

const short = (s, n = 60) => (s && s.length > n ? `${s.slice(0, n - 1)}…` : s || "");

// path, title, description and height let another page show someone else's
// graph, e.g. an organisation's end user (ADR 0030). path has no limit; the
// size picker adds it.
export default function Graph({ apiBase, auth, onOpenMemories, path = "/memory/graph", title = "Graph",
  description = "Your memories and the people, companies and conversations that connect them. Hover to see a neighbourhood, click for details.",
  height = "calc(100vh - 230px)" }) {
  const [data, setData] = useState(null);
  const [error, setError] = useState(null);
  const [size, setSize] = useState(SIZES[0]);
  const [showEntities, setShowEntities] = useState(true);
  const [showSessions, setShowSessions] = useState(true);
  const [selected, setSelected] = useState(null);
  const [query, setQuery] = useState("");
  const [laying, setLaying] = useState(false);
  const [colorBy, setColorBy] = useState("calm");
  const colorRef = useRef(colorBy);
  const ringsRef = useRef(null); // { focus, near: Set, far: Set } for hover or selection
  const matchRef = useRef(new Set()); // search hits, highlighted while typing
  const boxRef = useRef(null);
  const cardRef = useRef(null);
  // Full screen: the graph covers the whole window as an overlay, and also
  // asks the browser for real full screen. Browsers may refuse that (it needs
  // a direct click and can be blocked), and the overlay works either way.
  // Esc, the button, or leaving browser full screen exits.
  const [full, setFull] = useState(false);
  useEffect(() => {
    requestAnimationFrame(() => { sigmaRef.current?.resize(true); sigmaRef.current?.refresh(); });
    if (!full) return;
    const onKey = (e) => { if (e.key === "Escape") setFull(false); };
    const onChange = () => { if (!document.fullscreenElement) setFull(false); };
    window.addEventListener("keydown", onKey);
    document.addEventListener("fullscreenchange", onChange);
    return () => { window.removeEventListener("keydown", onKey); document.removeEventListener("fullscreenchange", onChange); };
  }, [full]);
  const toggleFull = useCallback(() => {
    if (full) {
      setFull(false);
      if (document.fullscreenElement) document.exitFullscreen().catch(() => {});
    } else {
      setFull(true);
      document.documentElement.requestFullscreen?.().catch(() => {}); // best effort
    }
  }, [full]);
  const sigmaRef = useRef(null);
  const hoverRef = useRef(null); // node under the pointer
  const selRef = useRef(null);
  const palette = usePalette();

  const load = useCallback(async () => {
    setError(null);
    setData(null);
    try {
      setData(await api(apiBase, `${path}${path.includes("?") ? "&" : "?"}limit=${size}`, { auth }));
    } catch (err) {
      setError(err.message || "Couldn't load the graph");
    }
  }, [apiBase, auth, path, size]);
  useEffect(() => { load(); }, [load]);

  // The model: memories and entities as nodes, sized by how connected they are.
  const model = useMemo(() => {
    if (!data) return null;
    const g = new GraphModel({ multi: false, type: "undirected" });
    for (const n of data.nodes) {
      if (!showEntities && n.kind === "entity") continue;
      g.addNode(n.id, { ...n, x: Math.random() * 100, y: Math.random() * 100 });
    }
    for (const l of data.links) {
      const s = endId(l.source), t = endId(l.target);
      if (s === t || !g.hasNode(s) || !g.hasNode(t) || g.hasEdge(s, t)) continue;
      if (!showSessions && l.type === "same_session") continue;
      g.addEdge(s, t, { type: "line", linkType: l.type });
    }
    g.forEachNode((id, a) => {
      const deg = g.degree(id);
      const entity = a.kind === "entity";
      g.mergeNodeAttributes(id, {
        size: entity ? Math.min(3.5 + Math.sqrt(deg) * 0.9, 10) : 2.5 + Math.min(Math.sqrt(deg), 2),
        color: entity ? palette.entity : palette.type[a.memory_type] || palette.muted,
        label: entity ? a.label : short(a.content || a.label),
        forceLabel: false, // set below for the most connected names
        zIndex: entity ? 1 : 0,
      });
    });
    // Only the 20 most connected people and companies are always labelled;
    // the rest show when zoomed in or highlighted.
    g.filterNodes((id, a) => a.kind === "entity").sort((x, y) => g.degree(y) - g.degree(x)).slice(0, 20)
      .forEach((id) => g.setNodeAttribute(id, "forceLabel", true));
    // Clusters: Louvain communities; the three biggest get a colour and are
    // named after their most connected person or company.
    const clusters = [];
    if (g.size > 0) {
      louvain.assign(g, { nodeCommunityAttribute: "community" });
      const bySize = {};
      g.forEachNode((id, a) => { (bySize[a.community] ||= []).push(id); });
      Object.entries(bySize).sort((x, y) => y[1].length - x[1].length).slice(0, 3).forEach(([community, ids], i) => {
        const hub = ids.filter((id) => g.getNodeAttribute(id, "kind") === "entity").sort((x, y) => g.degree(y) - g.degree(x))[0];
        clusters.push({ community: Number(community), color: palette.clusters[i], size: ids.length,
          name: hub ? g.getNodeAttribute(hub, "label") : `Group ${i + 1}`, hub });
      });
    }
    g.setAttribute("clusters", clusters);
    g.forEachNode((id, a) => {
      const c = clusters.find((x) => x.community === a.community);
      g.mergeNodeAttributes(id, { typeColor: a.color, clusterColor: c ? c.color : palette.muted, clusterHub: c?.hub === id });
    });
    g.forEachEdge((e, a) => {
      const c = palette.link[a.linkType] || palette.faint;
      // Sizes are pixels: thinner than ~1px and the line disappears.
      // Solid colours faded into the background: WebGL adds transparent lines
      // onto a dark background, so they would glare instead of fading.
      g.mergeEdgeAttributes(e, { color: mix(palette.bg, c, a.linkType === "mentions" ? 0.35 : a.linkType === "same_session" ? 0.45 : 0.8), size: a.linkType === "updates" || a.linkType === "causes" ? 2 : 1, zIndex: 0 });
    });
    return g;
  }, [data, showEntities, showSessions, palette]);

  // Renderer and layout. ForceAtlas2 runs in a worker so the page stays
  // responsive, then stops once the shape has settled.
  useEffect(() => {
    if (!model || !boxRef.current || model.order === 0) return;
    // A few synchronous passes give a sensible first frame.
    forceAtlas2.assign(model, { iterations: 50, settings: layoutSettings(model) });
    const renderer = new Sigma(model, boxRef.current, {
      renderEdgeLabels: false,
      labelFont: '"Geist", ui-sans-serif, system-ui, sans-serif',
      labelSize: 12,
      labelWeight: "600",
      labelColor: { color: palette.ink },
      labelDensity: 0.6,
      labelGridCellSize: 90,
      labelRenderedSizeThreshold: 9,
      defaultEdgeType: "line",
      zIndex: true,
      defaultDrawNodeHover: hoverDrawer(palette),
      minCameraRatio: 0.05,
      maxCameraRatio: 4,
      nodeReducer: (id, attrs) => {
        const mode = colorRef.current, cluster = mode === "cluster";
        const calm = attrs.kind === "entity" ? palette.calm.entity : palette.calm.memory;
        const base = { ...attrs, color: cluster ? attrs.clusterColor : mode === "type" ? attrs.typeColor : calm,
          ...(cluster && attrs.clusterHub ? { forceLabel: true, size: Math.max(attrs.size, 9), zIndex: 3 } : {}) };
        const rings = ringsRef.current;
        if (rings) {
          if (id === rings.focus) return { ...base, color: colorRef.current === "calm" ? palette.highlight : base.color, forceLabel: true, zIndex: 4, highlighted: true };
          if (rings.near.has(id)) return { ...base, color: colorRef.current === "calm" ? palette.highlight : base.color, forceLabel: true, zIndex: 3 };
          // The blast radius' second ring: present, but clearly behind the first.
          if (rings.far.has(id)) return { ...base, color: mix(base.color, palette.bg, 0.55), label: "", zIndex: 2 };
          return { ...base, color: mix(base.color, palette.bg, 0.85), label: "", zIndex: 0 };
        }
        if (matchRef.current.size) {
          return matchRef.current.has(id)
            // Memory hits can be hundreds: colour them all, label only people and
            // companies (the dropdown lists the memories).
            ? { ...base, color: palette.highlight, forceLabel: attrs.kind === "entity", label: attrs.kind === "entity" ? attrs.label : "", size: Math.max(attrs.size, 5), zIndex: 3 }
            : { ...base, color: mix(base.color, palette.bg, 0.85), label: "", zIndex: 0 };
        }
        return base;
      },
      edgeReducer: (id, attrs) => {
        const rings = ringsRef.current;
        const restEdge = colorRef.current === "calm" ? { ...attrs, color: palette.calm.edge } : attrs;
        if (!rings) return matchRef.current.size ? { ...attrs, hidden: true } : restEdge;
        const [a, b] = model.extremities(id);
        const touches = (n) => n === rings.focus || rings.near.has(n);
        if (touches(a) && touches(b) && (a === rings.focus || b === rings.focus)) {
          return { ...attrs, color: colorRef.current === "calm" ? mix(palette.bg, palette.highlight, 0.7) : mix(palette.bg, palette.link[attrs.linkType] || palette.muted, 0.9), size: Math.max(attrs.size, 1.4), zIndex: 2 };
        }
        if (touches(a) || touches(b)) return { ...attrs, color: mix(palette.bg, palette.muted, 0.25), zIndex: 1 };
        return { ...attrs, hidden: true };
      },
    });
    sigmaRef.current = renderer;
    const glowCanvas = document.createElement("canvas");
    glowCanvas.setAttribute("aria-hidden", "true");
    glowCanvas.style.cssText = "position:absolute;inset:0;width:100%;height:100%;pointer-events:none";
    boxRef.current.prepend(glowCanvas);
    renderer.on("afterRender", () => drawGlow(renderer, glowCanvas, model, palette, ringsRef.current?.focus));
    // Hover shows a node's neighbours; a selection keeps its blast radius
    // (two rings) until something else is chosen.
    const setRings = () => {
      const focus = hoverRef.current || selRef.current;
      ringsRef.current = focus && model.hasNode(focus) ? rings(model, focus, hoverRef.current ? 1 : 2) : null;
      renderer.refresh({ skipIndexation: true });
    };
    renderer.on("enterNode", ({ node }) => { hoverRef.current = node; boxRef.current.style.cursor = "pointer"; setRings(); });
    renderer.on("leaveNode", () => { hoverRef.current = null; boxRef.current.style.cursor = "default"; setRings(); });
    renderer.on("clickNode", ({ node }) => setSelected(node));
    renderer.on("clickStage", () => setSelected(null));

    const layout = new FA2Layout(model, { settings: layoutSettings(model) });
    layout.start();
    setLaying(true);
    const stop = setTimeout(() => { layout.stop(); setLaying(false); renderer.getCamera().animatedReset({ duration: 400 }); },
      Math.min(2000 + model.order * 2, 10000));
    return () => { clearTimeout(stop); layout.kill(); renderer.kill(); glowCanvas.remove(); sigmaRef.current = null; setLaying(false); };
  }, [model, palette]);

  // Selection lights up its neighbourhood and moves the camera to it.
  useEffect(() => {
    selRef.current = selected;
    const r = sigmaRef.current;
    if (!r) return;
    ringsRef.current = selected && model?.hasNode(selected) ? rings(model, selected, 2) : null;
    r.refresh({ skipIndexation: true });
    if (selected && model?.hasNode(selected)) {
      const pos = r.getNodeDisplayData(selected);
      if (pos) r.getCamera().animate({ x: pos.x, y: pos.y, ratio: Math.min(r.getCamera().ratio, 0.5) }, { duration: 600 });
    }
  }, [selected, model]);

  const fit = useCallback(() => sigmaRef.current?.getCamera().animatedReset({ duration: 500 }), []);
  const zoom = useCallback((f) => {
    const cam = sigmaRef.current?.getCamera();
    if (cam) f > 1 ? cam.animatedUnzoom({ duration: 250, factor: f }) : cam.animatedZoom({ duration: 250, factor: 1 / f });
  }, []);

  // Search: people, companies and memory text; Enter jumps to the best match.
  const matches = useMemo(() => {
    const q = query.trim().toLowerCase();
    if (!q || !model) return [];
    const out = [];
    model.forEachNode((id, a) => {
      const text = (a.kind === "entity" ? a.label : a.content || a.label || "").toLowerCase();
      if (text.includes(q)) out.push({ id, a, score: (a.kind === "entity" ? 2 : 0) + (text.startsWith(q) ? 1 : 0) + model.degree(id) / 1000 });
    });
    return out.sort((x, y) => y.score - x.score).slice(0, 8);
  }, [query, model]);

  // Every match is highlighted while typing (up to 500), not only the list's 8.
  useEffect(() => {
    const q = query.trim().toLowerCase();
    const hits = new Set();
    if (q && model) model.forEachNode((id, a) => {
      if (hits.size < 500 && (a.kind === "entity" ? a.label : a.content || a.label || "").toLowerCase().includes(q)) hits.add(id);
    });
    matchRef.current = hits;
    sigmaRef.current?.refresh({ skipIndexation: true });
  }, [query, model]);

  useEffect(() => { colorRef.current = colorBy; sigmaRef.current?.refresh({ skipIndexation: true }); }, [colorBy]);
  const clusters = model?.getAttribute("clusters") || [];

  const sel = selected && model?.hasNode(selected) ? model.getNodeAttributes(selected) : null;
  const neighbours = useMemo(() => {
    if (!sel || !model) return [];
    return model.edges(selected).slice(0, 10).map((e) => {
      const other = model.opposite(selected, e);
      return { type: model.getEdgeAttribute(e, "linkType"), id: other, node: model.getNodeAttributes(other) };
    });
  }, [sel, selected, model]);

  const memoryCount = data?.nodes.filter((n) => n.kind === "memory").length ?? 0;
  const entityCount = data?.nodes.filter((n) => n.kind === "entity").length ?? 0;
  const linkTypes = useMemo(() => [...new Set((data?.links || []).map((l) => l.type))], [data]);

  return (
    <div className="space-y-5">
      {(title || description) && (
        <PageHeader title={title} description={description}>
          <Toggle on={showEntities} onChange={setShowEntities}>People &amp; companies</Toggle>
          <Toggle on={showSessions} onChange={setShowSessions}>Conversations</Toggle>
        </PageHeader>
      )}

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
        <div ref={cardRef} className={full ? "fixed inset-0 z-[60] overflow-hidden" : "card relative overflow-hidden"}
          style={{ height: full ? "100dvh" : height, minHeight: 420, background: palette.bg }}>
          {/* Toolbar: search, size, view controls. */}
          <div className="absolute top-3 left-3 right-3 z-10 flex flex-wrap items-start gap-2">
            <div className="relative w-72 max-w-full">
              <Icon name="search" className="!text-[17px] absolute left-2.5 top-2 text-subtle pointer-events-none" />
              <input className="input h-9 w-full pl-8 bg-surface/95" value={query} placeholder="Find a person, company or memory"
                aria-label="Search the graph" onChange={(e) => setQuery(e.target.value)}
                onKeyDown={(e) => { if (e.key === "Enter" && matches[0]) { setSelected(matches[0].id); setQuery(""); } if (e.key === "Escape") setQuery(""); }} />
              {matches.length > 0 && (
                <ul className="absolute mt-1 w-full card p-1 shadow-[var(--shadow-pop)] max-h-72 overflow-y-auto">
                  {matches.map((m) => (
                    <li key={m.id}>
                      <button className="w-full text-left px-2 py-1.5 rounded-md hover:bg-panel text-[12.5px] flex gap-2 items-center"
                        onClick={() => { setSelected(m.id); setQuery(""); }}>
                        <span className="size-2 rounded-full shrink-0" style={{ background: m.a.color }} />
                        <span className="truncate">{m.a.kind === "entity" ? m.a.label : short(m.a.content, 70)}</span>
                      </button>
                    </li>
                  ))}
                </ul>
              )}
            </div>
            <div className="inline-flex items-center h-9 p-0.5 rounded-[10px] bg-surface/95 border border-line" role="radiogroup" aria-label="How many memories">
              {SIZES.map((n) => (
                <button key={n} role="radio" aria-checked={size === n} onClick={() => setSize(n)}
                  className={`h-full px-2.5 rounded-[8px] text-[12px] ${size === n ? "bg-panel text-ink font-medium border border-line" : "text-muted hover:text-ink border border-transparent"}`}>
                  {n.toLocaleString()}
                </button>
              ))}
            </div>
            <div className="inline-flex items-center h-9 p-0.5 rounded-[10px] bg-surface/95 border border-line" role="radiogroup" aria-label="Colour by">
              {[["calm", "Calm"], ["cluster", "Clusters"], ["type", "Types"]].map(([id, label]) => (
                <button key={id} role="radio" aria-checked={colorBy === id} onClick={() => setColorBy(id)}
                  className={`h-full px-2.5 rounded-[8px] text-[12px] ${colorBy === id ? "bg-panel text-ink font-medium border border-line" : "text-muted hover:text-ink border border-transparent"}`}>
                  {label}
                </button>
              ))}
            </div>
            <span className="flex-1" />
            <ToolButton icon="add" label="Zoom in" onClick={() => zoom(0.66)} />
            <ToolButton icon="remove" label="Zoom out" onClick={() => zoom(1.5)} />
            <ToolButton icon="center_focus_strong" label="Fit to view" onClick={fit} />
            <ToolButton icon="refresh" label="Reload" onClick={load} />
            <ToolButton icon={full ? "fullscreen_exit" : "fullscreen"} label={full ? "Exit full screen (Esc)" : "Full screen"} active={full} onClick={toggleFull} />
          </div>

          <div ref={boxRef} className="absolute inset-0" />
          {!data && <p className="absolute inset-0 grid place-items-center text-[13px] text-muted">Loading the graph…</p>}

          {/* Counts and legend. */}
          <div className="absolute bottom-3 left-4 z-10 space-y-1.5 max-w-[60%] pointer-events-none">
            <p className="text-[12px] text-muted font-mono">
              {data ? `${memoryCount.toLocaleString()} memories · ${entityCount.toLocaleString()} people & companies · ${data.links.length.toLocaleString()} links${data.truncated ? ` · newest ${size.toLocaleString()}` : ""}${laying ? " · arranging…" : ""}` : ""}
            </p>
            <div className="flex flex-wrap gap-x-4 gap-y-1 text-[11.5px] text-muted">
              {colorBy === "cluster" ? <>
                {clusters.map((c) => (
                  <span key={c.community} className="inline-flex items-center gap-1.5"><span className="size-2 rounded-full" style={{ background: c.color }} />{c.name} · {c.size.toLocaleString()}</span>
                ))}
                <span className="inline-flex items-center gap-1.5"><span className="size-2 rounded-full" style={{ background: palette.muted }} />Other clusters</span>
              </> : colorBy === "calm" ? <>
                <span className="inline-flex items-center gap-1.5"><span className="size-2 rounded-full" style={{ background: palette.calm.memory }} />Memory</span>
                <span className="inline-flex items-center gap-1.5"><span className="size-2.5 rounded-full" style={{ background: palette.calm.entity }} />Person or company</span>
                <span className="inline-flex items-center gap-1.5"><span className="size-2 rounded-full" style={{ background: palette.highlight, boxShadow: palette.dark ? `0 0 6px ${palette.glow}` : "none" }} />In focus</span>
              </> : Object.keys(TYPE_INFO).map((t) => (
                <span key={t} className="inline-flex items-center gap-1.5"><span className="size-2 rounded-full" style={{ background: palette.type[t] }} />{TYPE_INFO[t].label}</span>
              ))}
              {colorBy === "type" && showEntities && entityCount > 0 && (
                <span className="inline-flex items-center gap-1.5"><span className="size-2.5 rounded-full" style={{ background: palette.entity }} />Person or company</span>
              )}
              {colorBy !== "calm" && linkTypes.filter((t) => LINK_INFO[t] && (showSessions || t !== "same_session") && (showEntities || t !== "mentions")).map((t) => (
                <span key={t} className="inline-flex items-center gap-1.5"><span className="h-0.5 w-3.5 rounded-full" style={{ background: palette.link[t] }} />{LINK_INFO[t].label}</span>
              ))}
            </div>
          </div>

          {sel && (
            <aside aria-label="Selected node" className="absolute bottom-3 right-3 z-10 w-80 max-w-[calc(100%-24px)] card p-4 shadow-[var(--shadow-pop)]">
              <div className="flex items-start justify-between gap-3">
                <div className="min-w-0">
                  {sel.kind === "entity"
                    ? <span className="text-[12px] text-muted">Mentioned in {model.degree(selected)} memories</span>
                    : <span className="flex items-center gap-2 text-[12px] text-muted"><TypePill type={sel.memory_type} />{sel.at && formatDate(sel.at)}</span>}
                </div>
                <button onClick={() => setSelected(null)} aria-label="Close" className="text-subtle hover:text-ink"><Icon name="close" className="!text-[18px]" /></button>
              </div>
              <p className="mt-2 text-[13.5px] leading-relaxed text-ink line-clamp-6">{sel.kind === "entity" ? sel.label : sel.content || sel.label}</p>
              {neighbours.length > 0 && (
                <ul className="mt-3 pt-3 border-t border-line space-y-1.5 max-h-48 overflow-y-auto">
                  {neighbours.map(({ type, id, node }) => (
                    <li key={`${type}-${id}`}>
                      <button onClick={() => setSelected(id)} className="w-full text-left text-[12.5px] flex gap-2 hover:text-accent">
                        <span className="text-subtle shrink-0 w-[112px] truncate">{LINK_INFO[type]?.label || type}</span>
                        <span className="truncate text-ink">{node.kind === "entity" ? node.label : short(node.content || node.label, 50)}</span>
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
