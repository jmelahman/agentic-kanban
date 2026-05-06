import { useQuery } from "@tanstack/react-query";
import { ReactNode, useState } from "react";
import { api, PR_STATE_COLOR, PRState, Session } from "@/api/client";
import { queryKeys } from "@/api/keys";

export function InfoPanel({ session }: { session: Session }) {
  const portsQ = useQuery({
    queryKey: queryKeys.ports(session.id),
    queryFn: () => api.listPorts(session.id),
    refetchInterval: session.status === "running" ? 2000 : false,
  });
  const ports = portsQ.data ?? [];

  return (
    <div className="flex h-full flex-col gap-4 overflow-y-auto p-3 text-sm [scrollbar-gutter:stable]">
      <Section title="Session">
        <Row label="Status" value={<StatusValue status={session.status} />} />
        <Row label="Session ID" value={<Mono>{session.id}</Mono>} />
        <Row label="Started" value={formatTime(session.started_at)} />
        {session.stopped_at != null && (
          <Row label="Stopped" value={formatTime(session.stopped_at)} />
        )}
      </Section>

      <Section title="Container">
        <Row
          label="Name"
          value={
            session.container_name ? (
              <Copyable text={session.container_name} />
            ) : (
              <Muted>none</Muted>
            )
          }
        />
        <Row
          label="ID"
          value={
            session.container_id ? (
              <Copyable
                text={session.container_id}
                display={session.container_id.slice(0, 12)}
              />
            ) : (
              <Muted>none</Muted>
            )
          }
        />
      </Section>

      <Section title="Workspace">
        <Row label="Branch" value={<Copyable text={session.branch_name} />} />
        <Row
          label="Worktree"
          value={<Copyable text={session.worktree_path} />}
        />
      </Section>

      {session.pr_number != null && session.pr_url && (
        <Section title="Pull request">
          <Row
            label="PR"
            value={
              <a
                href={session.pr_url}
                target="_blank"
                rel="noreferrer"
                className={`hover:underline ${
                  session.pr_state
                    ? (PR_STATE_COLOR[session.pr_state as PRState] ??
                      "text-fg-muted")
                    : "text-fg-muted"
                }`}
              >
                #{session.pr_number}
                {session.pr_state ? ` (${session.pr_state})` : ""}
              </a>
            }
          />
        </Section>
      )}

      <Section title="Ports">
        {ports.length === 0 ? (
          <Muted>No ports allocated.</Muted>
        ) : (
          <ul className="flex flex-col gap-1">
            {ports.map((p) => (
              <li
                key={p.id}
                className="flex items-center justify-between rounded bg-surface px-2 py-1"
              >
                <div>
                  <div className="font-medium">{p.label}</div>
                  <div className="text-xs text-fg-muted">
                    container :{p.container_port} → host :{p.host_port}
                  </div>
                </div>
                {p.proxy_active ? (
                  <a
                    className="text-accent-500"
                    href={`http://localhost:${p.host_port}`}
                    target="_blank"
                    rel="noreferrer"
                  >
                    open ↗
                  </a>
                ) : (
                  <span className="text-xs text-fg-muted">inactive</span>
                )}
              </li>
            ))}
          </ul>
        )}
      </Section>
    </div>
  );
}

function Section({
  title,
  children,
}: {
  title: string;
  children: ReactNode;
}) {
  return (
    <section>
      <h3 className="mb-2 text-xs uppercase tracking-wide text-fg-muted">
        {title}
      </h3>
      <div className="flex flex-col gap-1">{children}</div>
    </section>
  );
}

function Row({ label, value }: { label: string; value: ReactNode }) {
  return (
    <div className="flex items-baseline gap-3">
      <span className="w-24 shrink-0 text-xs text-fg-muted">{label}</span>
      <span className="min-w-0 flex-1 break-all">{value}</span>
    </div>
  );
}

function Mono({ children }: { children: ReactNode }) {
  return <span className="font-mono text-xs">{children}</span>;
}

function Muted({ children }: { children: ReactNode }) {
  return <span className="text-fg-muted">{children}</span>;
}

function StatusValue({ status }: { status: string }) {
  const color =
    status === "running"
      ? "text-emerald-400"
      : status === "starting" || status === "stopping"
        ? "text-amber-400"
        : status === "error"
          ? "text-red-400"
          : "text-fg-muted";
  return <span className={color}>{status}</span>;
}

function Copyable({ text, display }: { text: string; display?: string }) {
  const [copied, setCopied] = useState(false);
  const onCopy = () => {
    navigator.clipboard.writeText(text);
    setCopied(true);
    setTimeout(() => setCopied(false), 1200);
  };
  return (
    <button
      onClick={onCopy}
      title={copied ? "copied" : "click to copy"}
      className="group inline-flex items-center gap-1 text-left font-mono text-xs hover:text-accent-500"
    >
      <span>{display ?? text}</span>
      <span className="text-fg-muted opacity-0 transition-opacity group-hover:opacity-100">
        {copied ? "✓" : "⧉"}
      </span>
    </button>
  );
}

function formatTime(ts?: number): ReactNode {
  if (ts == null) return <Muted>—</Muted>;
  const ms = ts < 1e12 ? ts * 1000 : ts;
  const d = new Date(ms);
  if (Number.isNaN(d.getTime())) return <Muted>—</Muted>;
  return <span title={d.toISOString()}>{d.toLocaleString()}</span>;
}
