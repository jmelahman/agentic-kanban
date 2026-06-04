import { useQuery } from "@tanstack/react-query";
import { useRef, useState } from "react";
import { api, type SessionSummary } from "@/api/client";
import { useClickOutside } from "@/hooks/useClickOutside";
import { ContainersIcon } from "@/icons";
import { Button } from "./Button";
import { STATUS_BG } from "./Ticket";

// Rows shown in the popover, in priority order. `idle`/`working`/`awaiting_perm`
// always render; `starting` only appears while something is spinning up.
const ROWS: { key: keyof SessionSummary; status: string; label: string; always: boolean }[] = [
  { key: "working", status: "working", label: "Working", always: true },
  { key: "awaiting_perm", status: "awaiting_perm", label: "Awaiting", always: true },
  { key: "idle", status: "idle", label: "Idle", always: true },
  { key: "starting", status: "starting", label: "Starting", always: false },
];

// SessionCounter is a header applet showing the instance-wide number of running
// containers, with a click-to-open popover breaking the total down by status.
// It polls the cheap /api/sessions/summary aggregate every few seconds.
export function SessionCounter() {
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement | null>(null);
  useClickOutside(ref, () => setOpen(false), { enabled: open });

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
    <div ref={ref} className="relative inline-flex">
      <Button
        variant="neutral"
        size="icon"
        className={`gap-1 lg:w-auto lg:px-2 lg:text-xs ${running === 0 ? "text-fg-muted" : ""}`}
        onClick={() => setOpen((v) => !v)}
        aria-expanded={open}
        aria-label="Running sessions"
        title={title}
      >
        <ContainersIcon />
        <span className="tabular-nums">{running}</span>
        <span className="hidden lg:inline">running</span>
      </Button>
      {open && (
        <div className="absolute right-0 top-full z-20 mt-1 w-44 overflow-hidden rounded border border-border bg-surface text-sm shadow-lg">
          {data && running > 0 ? (
            ROWS.filter((row) => row.always || data[row.key] > 0).map((row) => (
              <div key={row.key} className="flex items-center gap-2 px-3 py-1.5 text-fg">
                <span
                  aria-hidden
                  className={`h-2 w-2 rounded-full ${STATUS_BG[row.status] ?? "bg-fg-muted"}`}
                />
                <span className="flex-1 truncate text-fg-muted">{row.label}</span>
                <span className="tabular-nums">{data[row.key]}</span>
              </div>
            ))
          ) : (
            <div className="px-3 py-2 text-fg-muted">No running sessions</div>
          )}
        </div>
      )}
    </div>
  );
}
