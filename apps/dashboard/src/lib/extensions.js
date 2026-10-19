/**
 * Optional views a deployment adds to the dashboard without changing its
 * code. They are listed in /extensions.json (same origin):
 *
 *   {"views":[{"id":"…","label":"…","icon":"…","group":"…","path":"/…/"}]}
 *
 * Each view gets a navigation entry and opens its page in an iframe inside
 * the dashboard. A missing or invalid file means no extensions.
 */

const ID = /^[a-z0-9-]{1,40}$/;
const ICON = /^[a-z0-9_]{1,40}$/;

/** Returns a same-origin path, or null if `path` could leave the origin. */
export function safePath(path) {
  if (typeof path !== "string" || !path.startsWith("/") || path.startsWith("//") || path.includes("\\")) return null;
  try {
    const url = new URL(path, window.location.origin);
    return url.origin === window.location.origin ? url.pathname + url.search : null;
  } catch {
    return null;
  }
}

const text = (v, max) => (typeof v === "string" && v.trim() && v.length <= max ? v.trim() : null);

export function parseExtensions(json, reserved = []) {
  const views = Array.isArray(json?.views) ? json.views : [];
  const seen = new Set(reserved);
  const out = [];
  for (const v of views) {
    const id = typeof v?.id === "string" && ID.test(v.id) ? v.id : null;
    const label = text(v?.label, 40);
    const path = safePath(v?.path);
    if (!id || !label || !path || seen.has(id)) continue;
    seen.add(id);
    out.push({
      id,
      label,
      path,
      icon: typeof v.icon === "string" && ICON.test(v.icon) ? v.icon : "extension",
      group: text(v.group, 40) || "More",
    });
  }
  return out;
}

export async function loadExtensions(reserved) {
  try {
    const res = await fetch("/extensions.json", { headers: { Accept: "application/json" } });
    if (!res.ok) return [];
    return parseExtensions(await res.json(), reserved);
  } catch {
    return [];
  }
}

/** Adds extension views to nav groups: an existing group by label, else a new group at the end. */
export function withExtensions(nav, views) {
  const groups = nav.map((g) => ({ ...g, items: [...g.items] }));
  for (const v of views) {
    const item = { id: `x:${v.id}`, label: v.label, icon: v.icon, extension: v };
    const g = groups.find((x) => x.group.toLowerCase() === v.group.toLowerCase());
    if (g) g.items.push(item);
    else groups.push({ group: v.group, items: [item] });
  }
  return groups;
}
