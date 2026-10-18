import { Panel, Empty } from "../components/ui";

export default function Activity({ events }) {
  return (
    <div className="space-y-4">
      <div>
        <h1 className="text-[24px] font-semibold tracking-tight">Activity</h1>
        <p className="text-[13px] text-muted mt-1">What you did in this browser session. It isn't stored on the server.</p>
      </div>
      <Panel title={`${events.length} events`} bodyClass="p-0">
        {events.length === 0 ? (
          <Empty icon="history" title="Nothing yet">Searches, uploads and new memories will show up here.</Empty>
        ) : (
          <ul>
            {events.map((e) => (
              <li key={e.id} className="flex items-center gap-4 px-4 py-3 border-b border-line last:border-0 text-[13px]">
                <span className="font-mono text-[12px] text-subtle w-20 shrink-0">
                  {new Date(e.at).toLocaleTimeString(undefined, { hour: "2-digit", minute: "2-digit" })}
                </span>
                <span className={`size-1.5 rounded-full shrink-0 ${e.error ? "bg-danger" : "bg-ink"}`} />
                <span className="font-medium w-36 shrink-0 truncate">{e.title}</span>
                <span className="text-muted truncate">{e.detail}</span>
              </li>
            ))}
          </ul>
        )}
      </Panel>
    </div>
  );
}
