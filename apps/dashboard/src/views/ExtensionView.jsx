import { useEffect, useRef, useState } from "react";
import { Skeleton } from "../components/ui";

/**
 * Shows an extension's page (see lib/extensions.js) inside the dashboard.
 * The page is same-origin, so it shares the session; it is told the theme
 * with postMessage({type: "memro:theme", theme: "light" | "dark"}).
 */
export default function ExtensionView({ view, dark }) {
  const ref = useRef(null);
  const [loaded, setLoaded] = useState(false);
  const theme = dark ? "dark" : "light";
  const src = `${view.path}${view.path.includes("?") ? "&" : "?"}embed=1`;

  const post = () => ref.current?.contentWindow?.postMessage({ type: "memro:theme", theme }, window.location.origin);
  useEffect(post, [theme]); // eslint-disable-line react-hooks/exhaustive-deps

  return (
    <div className="relative -mx-4 md:-mx-8 -my-6 md:-my-8 h-[calc(100vh-4rem)]">
      {!loaded && (
        <div className="absolute inset-0 p-8 space-y-4" aria-hidden="true">
          <Skeleton className="h-8 w-48" /><Skeleton className="h-4 w-80" /><Skeleton className="h-48 w-full" />
        </div>
      )}
      <iframe
        ref={ref}
        key={view.id}
        src={src}
        title={view.label}
        className={`w-full h-full border-0 bg-canvas transition-opacity ${loaded ? "opacity-100" : "opacity-0"}`}
        onLoad={() => { setLoaded(true); post(); }}
      />
    </div>
  );
}
