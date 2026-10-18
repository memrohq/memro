import { useEffect } from "react";

export const Icon = ({ name, className = "" }) => (
  <span className={`icon ${className}`} aria-hidden="true">{name}</span>
);

export const Logo = ({ className = "size-5" }) => (
  <svg className={className} viewBox="0 0 32 32" fill="none" aria-hidden="true">
    <path d="M15 3.5 6.2 7.8 3.4 17.3l5.5 8.1 6.1 3V20l-3.6-2.6V12.6L15 10V3.5Z" stroke="currentColor" strokeWidth="2.2" strokeLinejoin="round" />
    <path d="M17 3.5l8.8 4.3 2.8 9.5-5.5 8.1-6.1 3v-6.6l5.2-3.8v-4.2L17 11V3.5Z" stroke="currentColor" strokeWidth="2.2" strokeLinejoin="round" />
    <circle cx="8.6" cy="12.6" r="1.6" fill="currentColor" />
    <circle cx="8.6" cy="17.6" r="1.6" fill="currentColor" />
  </svg>
);

/** Bordered section with a small monospace title bar, like the reference. */
export const Panel = ({ title, action, children, className = "", bodyClass = "p-4" }) => (
  <section className={`card bg-panel overflow-hidden ${className}`}>
    {title && (
      <header className="flex items-center justify-between gap-3 px-4 h-11">
        <h2 className="label">{title}</h2>
        {action}
      </header>
    )}
    <div className={`bg-surface border-t border-line rounded-xl -mx-px -mb-px ${bodyClass}`}>{children}</div>
  </section>
);

const TYPE_STYLES = {
  episodic: "bg-[#f1efff] text-[#5b4bc4] border-[#e2ddff]",
  semantic: "bg-[#edf5ff] text-[#2a62b8] border-[#d9e8ff]",
  procedural: "bg-[#ecf7f0] text-[#1f7a46] border-[#d5eedd]",
  profile: "bg-[#fdf4e6] text-[#9a5b12] border-[#f6e3c4]",
};

export const TypePill = ({ type }) => (
  <span className={`inline-flex items-center h-6 px-2 rounded-md border font-mono text-[11px] ${TYPE_STYLES[type] || "bg-panel text-muted border-line"}`}>
    {type}
  </span>
);

export const StatusPill = ({ ok, okText = "Online", badText = "Offline", warn }) => {
  const color = ok ? "text-ok" : warn ? "text-warn" : "text-danger";
  return (
    <span className="inline-flex items-center gap-1.5 h-6 px-2 rounded-full border border-line bg-surface text-[12px] text-ink">
      <span className={`size-1.5 rounded-full bg-current ${color}`} />
      {ok ? okText : badText}
    </span>
  );
};

export const Modal = ({ open, title, onClose, children, width = "max-w-lg" }) => {
  useEffect(() => {
    if (!open) return;
    const onKey = (e) => e.key === "Escape" && onClose();
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [open, onClose]);
  if (!open) return null;
  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/25 p-4" onClick={onClose}>
      <div
        role="dialog"
        aria-modal="true"
        className={`card w-full ${width} shadow-[0_20px_60px_-20px_rgba(0,0,0,0.35)]`}
        onClick={(e) => e.stopPropagation()}
      >
        <header className="flex items-center justify-between px-5 h-14 border-b border-line">
          <h3 className="text-[15px] font-semibold">{title}</h3>
          <button className="btn-ghost size-8 px-0" onClick={onClose} aria-label="Close">
            <Icon name="close" />
          </button>
        </header>
        <div className="p-5">{children}</div>
      </div>
    </div>
  );
};

export const Field = ({ label, hint, children }) => (
  <label className="block">
    <span className="block text-[12px] font-medium text-ink mb-1.5">{label}</span>
    {children}
    {hint && <span className="block text-[12px] text-subtle mt-1.5">{hint}</span>}
  </label>
);

export const Empty = ({ icon = "inbox", title, children }) => (
  <div className="flex flex-col items-center justify-center text-center py-14 px-6">
    <div className="size-10 rounded-full bg-panel border border-line flex items-center justify-center mb-3">
      <Icon name={icon} className="text-muted" />
    </div>
    <p className="text-[14px] font-medium">{title}</p>
    {children && <p className="text-[13px] text-muted mt-1 max-w-sm">{children}</p>}
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
