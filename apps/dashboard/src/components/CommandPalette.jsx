import { useEffect, useMemo, useRef, useState } from "react";
import { Icon } from "./ui";

/**
 * ⌘K palette: jump to a page, run an action, or search memories for the
 * typed text. `commands` is [{ id, label, icon, group, hint?, run }].
 */
export default function CommandPalette({ open, onClose, commands, onSearch }) {
  const [query, setQuery] = useState("");
  const [active, setActive] = useState(0);
  const inputRef = useRef(null);
  const listRef = useRef(null);

  useEffect(() => {
    if (open) {
      setQuery("");
      setActive(0);
      requestAnimationFrame(() => inputRef.current?.focus());
    }
  }, [open]);

  const items = useMemo(() => {
    const q = query.trim().toLowerCase();
    const matched = commands.filter((c) => !q || c.label.toLowerCase().includes(q) || c.group.toLowerCase().includes(q));
    if (q) {
      matched.unshift({ id: "search", label: `Search memories for “${query.trim()}”`, icon: "search", group: "Search", run: () => onSearch(query.trim()) });
    }
    return matched;
  }, [commands, query, onSearch]);

  useEffect(() => setActive(0), [query]);
  useEffect(() => {
    listRef.current?.querySelector(`[data-index="${active}"]`)?.scrollIntoView({ block: "nearest" });
  }, [active]);

  if (!open) return null;

  const run = (item) => { onClose(); item?.run(); };
  const onKey = (e) => {
    if (e.key === "ArrowDown") { e.preventDefault(); setActive((a) => Math.min(a + 1, items.length - 1)); }
    else if (e.key === "ArrowUp") { e.preventDefault(); setActive((a) => Math.max(a - 1, 0)); }
    else if (e.key === "Enter") { e.preventDefault(); run(items[active]); }
    else if (e.key === "Escape") { e.preventDefault(); onClose(); }
  };

  let lastGroup = null;
  return (
    <div className="fixed inset-0 z-[60] flex items-start justify-center bg-ink/20 dark:bg-black/60 backdrop-blur-[3px] p-4 pt-[14vh] fade-in" onClick={onClose}>
      <div role="dialog" aria-modal="true" aria-label="Command palette" className="w-full max-w-[560px] bg-elevated rounded-2xl shadow-pop overflow-hidden pop-in" onClick={(e) => e.stopPropagation()}>
        <div className="flex items-center gap-3 px-4 h-14 border-b border-line">
          <Icon name="search" className="text-subtle" />
          <input
            ref={inputRef}
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            onKeyDown={onKey}
            placeholder="Search memories or jump to…"
            className="flex-1 bg-transparent outline-none text-[15px] placeholder:text-subtle"
            aria-label="Command"
          />
          <span className="kbd">esc</span>
        </div>
        <ul ref={listRef} className="max-h-[360px] overflow-y-auto scroll-thin p-2" role="listbox">
          {items.length === 0 && <li className="px-3 py-8 text-center text-[13px] text-muted">Nothing matches.</li>}
          {items.map((item, i) => {
            const header = item.group !== lastGroup ? item.group : null;
            lastGroup = item.group;
            return (
              <li key={item.id}>
                {header && <p className="label px-3 pt-3 pb-1.5">{header}</p>}
                <button
                  data-index={i}
                  role="option"
                  aria-selected={i === active}
                  onMouseMove={() => setActive(i)}
                  onClick={() => run(item)}
                  className={`w-full flex items-center gap-3 h-10 px-3 rounded-[10px] text-[13.5px] text-left ${i === active ? "bg-accent-soft text-ink" : "text-ink"}`}
                >
                  <Icon name={item.icon} className={i === active ? "text-accent" : "text-subtle"} />
                  <span className="flex-1 truncate">{item.label}</span>
                  {item.hint && <span className="text-[12px] text-subtle">{item.hint}</span>}
                  {i === active && <Icon name="keyboard_return" className="text-subtle !text-[16px]" />}
                </button>
              </li>
            );
          })}
        </ul>
        <div className="flex items-center gap-4 px-4 h-10 border-t border-line text-[11.5px] text-subtle">
          <span className="flex items-center gap-1.5"><span className="kbd">↑</span><span className="kbd">↓</span> navigate</span>
          <span className="flex items-center gap-1.5"><span className="kbd">↵</span> open</span>
        </div>
      </div>
    </div>
  );
}
