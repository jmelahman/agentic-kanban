import { useQuery } from "@tanstack/react-query";
import { api, type SessionSummary } from "@/api/client";
import { STATUS_BG } from "./Ticket";

// Status breakdown rendered inline in the header. `working`/`awaiting_perm`/`idle`
// always render; `starting` only appears while something is spinning up.
const ROWS: { key: keyof SessionSummary; status: string; label: string; always: boolean }[] = [
  { key: "working", status: "working", label: "working", always: true },
  { key: "awaiting_perm", status: "awaiting_perm", label: "awaiting", always: true },
  { key: "idle", status: "idle", label: "idle", always: true },
  { key: "starting", status: "starting", label: "starting", always: false },
];

// SessionCounter is a header applet showing the instance-wide number of running
// containers broken down by status: a colored status dot next to each count.
// It polls the cheap /api/sessions/summary aggregate every few seconds.
export function SessionCounter() {
  const { data } = useQuery({
    queryKey: ["session-summary"],
    queryFn: api.sessionSummary,
    refetchInterval: 3000,
  });

  const running = data?.running ?? 0;
  const title =
    running === 0
      ? "No running sessions"
      : `${running} running — ${data?.working ?? 0} working, ${data?.awaiting_perm ?? 0} awaiting, ${data?.idle ?? 0} idle`;

  return (
    <div className="mr-2 inline-flex items-center gap-2.5 text-xs tabular-nums" title={title}>
      {ROWS.filter((row) => row.always || (data?.[row.key] ?? 0) > 0).map((row) => {
        const count = data?.[row.key] ?? 0;
        return (
          <span
            key={row.key}
            className={`inline-flex items-center gap-1 ${count === 0 ? "text-fg-muted" : "text-fg"}`}
          >
            <span
              aria-hidden
              className={`h-2 w-2 rounded-full ${STATUS_BG[row.status] ?? "bg-fg-muted"} ${
                count === 0 ? "opacity-40" : ""
              }`}
            />
            <span className="sr-only">{row.label} </span>
            {count}
          </span>
        );
      })}
    </div>
  );
}
