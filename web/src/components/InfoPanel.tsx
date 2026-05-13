import { useMutation, useQuery } from "@tanstack/react-query";
import { type ReactNode, useEffect, useRef, useState } from "react";
import {
  api,
  PR_STATE_COLOR,
  type PRDetail,
  type PRReviewDecision,
  type PRState,
  type Session,
} from "@/api/client";
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
        {session.claude_session_id && (
          <Row label="Claude session" value={<Copyable text={session.claude_session_id} />} />
        )}
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
              <Copyable text={session.container_id} display={session.container_id.slice(0, 12)} />
            ) : (
              <Muted>none</Muted>
            )
          }
        />
      </Section>

      <Section title="Workspace">
        <Row label="Branch" value={<Copyable text={session.branch_name} />} />
        <Row label="Worktree" value={<Copyable text={session.worktree_path} />} />
      </Section>

      {session.pr_number != null && session.pr_url && <PRSection session={session} />}

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

function DescriptionEditor({ ticketId, body }: { ticketId: number; body: string }) {
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
      <p className="text-xs text-fg-muted">⌘/Ctrl + Enter to save · Esc to cancel</p>
    </div>
  );
}

function autosize(el: HTMLTextAreaElement) {
  el.style.height = "auto";
  el.style.height = `${el.scrollHeight}px`;
}

function Section({ title, children }: { title: string; children: ReactNode }) {
  return (
    <section>
      <h3 className="mb-2 text-xs uppercase tracking-wide text-fg-muted">{title}</h3>
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

function PRSection({ session }: { session: Session }) {
  const detailQ = useQuery({
    queryKey: queryKeys.prDetail(session.id),
    queryFn: () => api.prDetail(session.id),
    enabled: session.pr_number != null,
    staleTime: 30_000,
    refetchInterval: session.status === "running" ? 60_000 : false,
    retry: false,
  });
  const detail = detailQ.data;

  return (
    <Section title="Pull request">
      <Row
        label="PR"
        value={
          <div className="flex min-w-0 items-baseline gap-2">
            <a
              href={session.pr_url}
              target="_blank"
              rel="noreferrer"
              className={`min-w-0 truncate hover:underline ${
                session.pr_state
                  ? (PR_STATE_COLOR[session.pr_state as PRState] ?? "text-fg-muted")
                  : "text-fg-muted"
              }`}
              title={session.pr_title ?? ""}
            >
              #{session.pr_number}
              {session.pr_title ? ` ${session.pr_title}` : ""}
              {session.pr_state ? ` (${session.pr_state})` : ""}
            </a>
            <CopyPRLink session={session} detail={detail} />
          </div>
        }
      />
      <Row label="Review" value={<ReviewValue detail={detail} loading={detailQ.isLoading} />} />
      <Row label="Checks" value={<ChecksValue detail={detail} loading={detailQ.isLoading} />} />
    </Section>
  );
}

function ReviewValue({ detail, loading }: { detail: PRDetail | undefined; loading: boolean }) {
  if (loading && !detail) return <Muted>…</Muted>;
  if (!detail) return <Muted>—</Muted>;
  return reviewBadge(detail.review_decision);
}

function reviewBadge(decision: PRReviewDecision): ReactNode {
  switch (decision) {
    case "approved":
      return <span className="text-emerald-400">✓ Approved</span>;
    case "changes_requested":
      return <span className="text-red-400">✗ Changes requested</span>;
    case "review_required":
      return <span className="text-fg-muted">· Review pending</span>;
    default:
      return <Muted>—</Muted>;
  }
}

function ChecksValue({ detail, loading }: { detail: PRDetail | undefined; loading: boolean }) {
  if (loading && !detail) return <Muted>…</Muted>;
  if (!detail) return <Muted>—</Muted>;
  const c = detail.checks;
  if (c.total === 0) return <Muted>none</Muted>;
  const parts: ReactNode[] = [];
  if (c.success > 0) {
    parts.push(
      <span key="ok" className="text-emerald-400">
        ✓ {c.success}
      </span>,
    );
  }
  if (c.failure > 0) {
    parts.push(
      <span
        key="fail"
        className="cursor-help text-red-400"
        title={
          c.failing.length > 0
            ? `Failing:\n${c.failing.map((f) => `• ${f.name}`).join("\n")}`
            : `${c.failure} failing`
        }
      >
        ✗ {c.failure}
      </span>,
    );
  }
  if (c.pending > 0) {
    parts.push(
      <span key="pend" className="text-amber-400">
        · {c.pending}
      </span>,
    );
  }
  return (
    <span className="inline-flex items-center gap-2">
      {parts.flatMap((p, i) =>
        i === 0
          ? [p]
          : [
              <span key={`sep-${i}`} className="text-fg-muted">
                /
              </span>,
              p,
            ],
      )}
    </span>
  );
}

function CopyPRLink({ session, detail }: { session: Session; detail: PRDetail | undefined }) {
  const [copied, setCopied] = useState(false);
  const [busy, setBusy] = useState(false);

  if (!session.pr_url) return null;

  const onCopy = async () => {
    if (busy) return;
    setBusy(true);
    try {
      const url = session.pr_url!;
      const title =
        session.pr_title && session.pr_title.trim().length > 0
          ? session.pr_title
          : `#${session.pr_number}`;
      let stats = detail;
      if (!stats) {
        try {
          stats = await api.prDetail(session.id);
        } catch {
          // diff is optional — fall back to a link without it
        }
      }
      const diff = stats ? ` (+${stats.additions} / -${stats.deletions})` : "";
      const html = `<a href="${escapeHtml(url)}">${escapeHtml(title)}</a>${escapeHtml(diff)}`;
      const text = `${title}${diff} ${url}`;
      const w = window as Window & typeof globalThis & { ClipboardItem?: typeof ClipboardItem };
      if (w.ClipboardItem && navigator.clipboard?.write) {
        await navigator.clipboard.write([
          new w.ClipboardItem({
            "text/html": new Blob([html], { type: "text/html" }),
            "text/plain": new Blob([text], { type: "text/plain" }),
          }),
        ]);
      } else {
        await navigator.clipboard.writeText(text);
      }
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
    } finally {
      setBusy(false);
    }
  };

  return (
    <button
      type="button"
      onClick={onCopy}
      disabled={busy}
      title={copied ? "copied" : "copy link for Slack"}
      className="shrink-0 text-fg-muted hover:text-accent-500 disabled:opacity-50"
    >
      {copied ? "✓" : "⧉"}
    </button>
  );
}

function escapeHtml(s: string): string {
  return s
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;")
    .replace(/'/g, "&#39;");
}

function formatTime(ts?: number): ReactNode {
  if (ts == null) return <Muted>—</Muted>;
  const ms = ts < 1e12 ? ts * 1000 : ts;
  const d = new Date(ms);
  if (Number.isNaN(d.getTime())) return <Muted>—</Muted>;
  return (
    <span className="text-xs" title={d.toISOString()}>
      {d.toLocaleString()}
    </span>
  );
}
