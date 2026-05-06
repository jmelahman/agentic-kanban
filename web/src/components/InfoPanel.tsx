import { useMutation, useQuery } from "@tanstack/react-query";
import { ReactNode, useEffect, useRef, useState } from "react";
import { api, PR_STATE_COLOR, PRState, Session } from "@/api/client";
import { queryKeys } from "@/api/keys";
import { ticketStore, useTicket } from "@/store";

export function InfoPanel({ session }: { session: Session }) {
  const ticket = useTicket(session.ticket_id);
  const portsQ = useQuery({
    queryKey: queryKeys.ports(session.id),
    queryFn: () => api.listPorts(session.id),
    refetchInterval: session.status === "running" ? 2000 : false,
  });
  const ports = portsQ.data ?? [];

  return (
    <div className="flex h-full flex-col gap-4 overflow-y-auto p-3 text-sm [scrollbar-gutter:stable]">
      <Section title="Description">
        <DescriptionEditor ticketId={session.ticket_id} body={ticket?.body ?? ""} />
      </Section>

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

function DescriptionEditor({
  ticketId,
  body,
}: {
  ticketId: number;
  body: string;
}) {
  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState(body);
  const textareaRef = useRef<HTMLTextAreaElement | null>(null);

  const saveMut = useMutation({
    mutationFn: (next: string) => api.updateTicket(ticketId, { body: next }),
    onSuccess: (updated) => {
      ticketStore.set(updated.id, updated);
      setEditing(false);
    },
  });

  useEffect(() => {
    if (!editing) return;
    setDraft(body);
    requestAnimationFrame(() => {
      const el = textareaRef.current;
      if (!el) return;
      el.focus();
      el.setSelectionRange(el.value.length, el.value.length);
      autosize(el);
    });
  }, [editing, body]);

  if (!editing) {
    return (
      <button
        type="button"
        onClick={() => setEditing(true)}
        className="-mx-1 rounded px-1 py-0.5 text-left hover:bg-surface"
        title="Click to edit"
      >
        {body ? (
          <p className="whitespace-pre-wrap break-words">{body}</p>
        ) : (
          <Muted>No description. Click to add.</Muted>
        )}
      </button>
    );
  }

  const submit = () => {
    if (draft === body) {
      setEditing(false);
      return;
    }
    saveMut.mutate(draft);
  };

  return (
    <div className="flex flex-col gap-1">
      <textarea
        ref={textareaRef}
        value={draft}
        onChange={(e) => {
          setDraft(e.target.value);
          autosize(e.currentTarget);
        }}
        onKeyDown={(e) => {
          if (e.key === "Escape") {
            e.preventDefault();
            setEditing(false);
          } else if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) {
            e.preventDefault();
            submit();
          }
        }}
        onBlur={submit}
        disabled={saveMut.isPending}
        placeholder="Add a description…"
        className="min-h-[6rem] w-full resize-none rounded bg-surface px-2 py-1 outline-none ring-1 ring-border focus:ring-accent-500"
      />
      <p className="text-xs text-fg-muted">
        ⌘/Ctrl + Enter to save · Esc to cancel
      </p>
    </div>
  );
}

function autosize(el: HTMLTextAreaElement) {
  el.style.height = "auto";
  el.style.height = `${el.scrollHeight}px`;
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
