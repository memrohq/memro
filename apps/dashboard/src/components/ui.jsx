import { useEffect } from "react";

export const Icon = ({ name, className = "", fill = false }) => (
  <span className={`icon ${fill ? "icon-fill" : ""} ${className}`} aria-hidden="true">{name}</span>
);

/** The Memro "M" mark (memrologo), as an image so it keeps its colors. */
export const Logo = ({ className = "size-5" }) => (
  <img src="/memro-mark.png" alt="" aria-hidden="true" className={`${className} object-contain`} />
);

/** The full "memro.co" wordmark, with light text on dark surfaces. */
export const Wordmark = ({ className = "h-6" }) => (
  <span className="inline-flex self-start shrink-0">
    <img src="/memro-wordmark-dark.png" alt="memro.co" className={`${className} w-auto max-w-none object-contain dark:hidden`} />
    <img src="/memro-wordmark-light.png" alt="memro.co" className={`${className} w-auto max-w-none object-contain hidden dark:block`} />
  </span>
);

/** Title row at the top of every page. */
export const PageHeader = ({ title, description, children }) => (
  <div className="flex flex-wrap items-end justify-between gap-x-4 gap-y-3">
    <div className="min-w-0">
      <h1 className="text-[26px] leading-tight font-semibold tracking-[-0.02em]">{title}</h1>
      {description && <p className="text-[14px] text-muted mt-1.5 max-w-2xl">{description}</p>}
    </div>
    {children && <div className="flex flex-wrap items-center gap-2">{children}</div>}
  </div>
);

/** A card with an optional header (title, description, action). */
export const Panel = ({ title, description, action, children, className = "", bodyClass = "p-5" }) => (
  <section className={`card overflow-hidden ${className}`}>
    {(title || action) && (
      <header className="flex items-start justify-between gap-3 px-5 pt-4 pb-3 border-b border-line">
        <div className="min-w-0">
          {title && <h2 className="text-[14px] font-semibold tracking-[-0.01em]">{title}</h2>}
          {description && <p className="text-[12.5px] text-muted mt-0.5">{description}</p>}
        </div>
        {action}
      </header>
    )}
    <div className={bodyClass}>{children}</div>
  </section>
);

export const MEMORY_TYPES = ["episodic", "semantic", "procedural", "profile"];

export const TYPE_INFO = {
  episodic: { label: "Episodic", hint: "What happened", fill: "bg-episodic" },
  semantic: { label: "Semantic", hint: "What's true", fill: "bg-semantic" },
  procedural: { label: "Procedural", hint: "How to act", fill: "bg-procedural" },
  profile: { label: "Profile", hint: "Who they are", fill: "bg-profile" },
};

/** Solid fill per memory type, for bars and dots. */
export const TYPE_FILL = Object.fromEntries(Object.entries(TYPE_INFO).map(([k, v]) => [k, v.fill]));

/** Memory type: a colored dot carries identity, the label stays in ink. */
export const TypePill = ({ type }) => (
  <span className="inline-flex items-center gap-1.5 h-6 pl-2 pr-2.5 rounded-full border border-line bg-surface text-[12px] text-ink whitespace-nowrap">
    <span className={`size-2 rounded-full ${TYPE_FILL[type] || "bg-subtle"}`} />
    {TYPE_INFO[type]?.label || type}
  </span>
);

export const StatusPill = ({ ok, okText = "Online", badText = "Offline", warn }) => {
  const color = ok ? "bg-ok" : warn ? "bg-warn" : "bg-danger";
  return (
    <span className="inline-flex items-center gap-1.5 h-6 px-2.5 rounded-full border border-line bg-surface text-[12px] text-ink whitespace-nowrap">
      <span className="relative flex size-2">
        {ok && <span className={`absolute inline-flex size-full rounded-full ${color} opacity-40 animate-ping`} />}
        <span className={`relative inline-flex size-2 rounded-full ${color}`} />
      </span>
      {ok ? okText : badText}
    </span>
  );
};

function useEscape(open, onClose) {
  useEffect(() => {
    if (!open) return;
    const onKey = (e) => e.key === "Escape" && onClose();
    window.addEventListener("keydown", onKey);
    const prev = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    return () => { window.removeEventListener("keydown", onKey); document.body.style.overflow = prev; };
  }, [open, onClose]);
}

export const Modal = ({ open, title, description, onClose, children, width = "max-w-lg" }) => {
  useEscape(open, onClose);
  if (!open) return null;
  return (
    <div className="fixed inset-0 z-50 flex items-start sm:items-center justify-center bg-ink/20 dark:bg-black/60 backdrop-blur-[3px] p-4 pt-[12vh] sm:pt-4 fade-in" onClick={onClose}>
      <div role="dialog" aria-modal="true" aria-label={title} className={`w-full ${width} bg-elevated rounded-2xl shadow-pop pop-in`} onClick={(e) => e.stopPropagation()}>
        <header className="flex items-start justify-between gap-3 px-6 pt-5 pb-1">
          <div>
            <h3 className="text-[16px] font-semibold tracking-[-0.01em]">{title}</h3>
            {description && <p className="text-[13px] text-muted mt-1">{description}</p>}
          </div>
          <button className="btn-ghost size-8 px-0 -mr-2 -mt-1" onClick={onClose} aria-label="Close"><Icon name="close" /></button>
        </header>
        <div className="px-6 pb-6 pt-4">{children}</div>
      </div>
    </div>
  );
};

/** A panel that slides in from the right, for details. */
export const Drawer = ({ open, title, onClose, children, footer }) => {
  useEscape(open, onClose);
  if (!open) return null;
  return (
    <div className="fixed inset-0 z-50 flex justify-end bg-ink/15 dark:bg-black/50 backdrop-blur-[2px] fade-in" onClick={onClose}>
      <aside role="dialog" aria-modal="true" aria-label={title} className="h-full w-full max-w-[520px] bg-elevated border-l border-line shadow-pop flex flex-col slide-in" onClick={(e) => e.stopPropagation()}>
        <header className="flex items-center justify-between gap-3 px-6 h-16 border-b border-line shrink-0">
          <h3 className="text-[15px] font-semibold truncate">{title}</h3>
          <button className="btn-ghost size-8 px-0 -mr-2" onClick={onClose} aria-label="Close"><Icon name="close" /></button>
        </header>
        <div className="flex-1 overflow-y-auto scroll-thin px-6 py-5">{children}</div>
        {footer && <footer className="px-6 py-4 border-t border-line shrink-0 flex justify-end gap-2">{footer}</footer>}
      </aside>
    </div>
  );
};

export const Field = ({ label, hint, children }) => (
  <label className="block">
    <span className="block text-[12.5px] font-medium text-ink mb-1.5">{label}</span>
    {children}
    {hint && <span className="block text-[12px] text-subtle mt-1.5">{hint}</span>}
  </label>
);

export const Empty = ({ icon = "inbox", title, children, action }) => (
  <div className="flex flex-col items-center justify-center text-center py-16 px-6">
    <div className="relative mb-4">
      <div className="absolute inset-0 rounded-2xl bg-accent/20 blur-xl" />
      <div className="relative size-12 rounded-2xl bg-gradient-to-b from-surface to-panel border border-line shadow-card flex items-center justify-center">
        <Icon name={icon} className="text-accent !text-[22px]" />
      </div>
    </div>
    <p className="text-[15px] font-semibold tracking-[-0.01em]">{title}</p>
    {children && <p className="text-[13px] text-muted mt-1.5 max-w-sm">{children}</p>}
    {action && <div className="mt-5">{action}</div>}
  </div>
);

export const Skeleton = ({ className = "h-4 w-24" }) => <span className={`skeleton block ${className}`} />;

/** A row of mutually exclusive options. */
export const Segmented = ({ options, value, onChange, size = "md" }) => (
  <div className={`inline-flex items-center p-0.5 rounded-[10px] bg-panel border border-line ${size === "sm" ? "h-8" : "h-9"}`} role="tablist">
    {options.map((o) => {
      const id = typeof o === "string" ? o : o.id;
      const label = typeof o === "string" ? o : o.label;
      const active = value === id;
      return (
        <button
          key={id}
          role="tab"
          aria-selected={active}
          onClick={() => onChange(id)}
          className={`h-full px-3 rounded-[8px] text-[12.5px] capitalize transition-colors ${
            active ? "bg-surface text-ink font-medium shadow-card border border-line" : "text-muted hover:text-ink border border-transparent"
          }`}
        >
          {label}
        </button>
      );
    })}
  </div>
);

export const timeAgo = (iso) => {
  if (!iso) return "—";
  const s = Math.max(0, (Date.now() - new Date(iso).getTime()) / 1000);
  if (s < 60) return "just now";
  if (s < 3600) return `${Math.floor(s / 60)}m ago`;
  if (s < 86400) return `${Math.floor(s / 3600)}h ago`;
  if (s < 86400 * 7) return `${Math.floor(s / 86400)}d ago`;
  return new Date(iso).toLocaleDateString(undefined, { day: "numeric", month: "short", year: "numeric" });
};

export const formatDate = (iso) =>
  iso ? new Date(iso).toLocaleString(undefined, { day: "numeric", month: "short", year: "numeric", hour: "2-digit", minute: "2-digit" }) : "—";
