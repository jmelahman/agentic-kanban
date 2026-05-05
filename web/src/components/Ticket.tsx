import { useDraggable } from "@dnd-kit/core";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useEffect, useRef, useState } from "react";
import { api, Session, Ticket as TicketType } from "@/api/client";
import { queryKeys } from "@/api/keys";

const STATUS_COLOR: Record<string, string> = {
  stopped: "text-fg-muted",
  starting: "text-amber-400",
  stopping: "text-amber-400",
  idle: "text-emerald-400",
  working: "text-sky-400",
  awaiting_perm: "text-yellow-400",
  error: "text-danger",
};

function TicketCard({
  ticket,
  session,
  active,
  onClick,
  onDoubleClick,
  className,
  innerRef,
  style,
  dragProps,
  interactive = true,
  titleSlot,
}: {
  ticket: TicketType;
  session: Session | null;
  active: boolean;
  onClick?: () => void;
  onDoubleClick?: React.MouseEventHandler<HTMLDivElement>;
  className?: string;
  innerRef?: (el: HTMLElement | null) => void;
  style?: React.CSSProperties;
  dragProps?: React.HTMLAttributes<HTMLDivElement>;
  interactive?: boolean;
  titleSlot?: React.ReactNode;
}) {
  const status = session?.status ?? "stopped";
  const interactiveCls = interactive ? "cursor-pointer hover:bg-surface-3" : "";
  return (
    <div
      ref={innerRef}
      {...dragProps}
      style={style}
      onClick={onClick}
      onDoubleClick={onDoubleClick}
      data-ticket-card="true"
      className={`rounded bg-surface-2 p-2 text-sm transition-colors duration-150 ${interactiveCls} ${active ? "ring-2 ring-accent-500" : ""} ${className ?? ""}`}
    >
      <div className="flex items-center justify-between gap-2">
        {titleSlot ?? <span className="font-medium">{ticket.title}</span>}
        <span className={`text-xs ${STATUS_COLOR[status] ?? "text-fg-muted"}`}>{status}</span>
      </div>
      {ticket.body && <p className="mt-1 text-xs text-fg-muted line-clamp-2">{ticket.body}</p>}
    </div>
  );
}

export function Ticket({
  ticket,
  session,
  active,
  onSelect,
}: {
  ticket: TicketType;
  session: Session | null;
  active: boolean;
  onSelect: () => void;
}) {
  const qc = useQueryClient();
  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState(ticket.title);
  const inputRef = useRef<HTMLInputElement>(null);

  const { attributes, listeners, setNodeRef, isDragging } = useDraggable({
    id: ticket.id,
    disabled: editing,
  });
  const style: React.CSSProperties = {
    opacity: isDragging ? 0.5 : 1,
  };

  const renameMut = useMutation({
    mutationFn: (title: string) => api.updateTicket(ticket.id, { title }),
    onSuccess: () => {
      setEditing(false);
      qc.invalidateQueries({ queryKey: queryKeys.board(ticket.board_id) });
    },
  });

  useEffect(() => {
    if (editing) {
      setDraft(ticket.title);
      requestAnimationFrame(() => {
        inputRef.current?.focus();
        inputRef.current?.select();
      });
    }
  }, [editing, ticket.title]);

  const submit = () => {
    const next = draft.trim();
    if (!next) return;
    if (next === ticket.title) {
      setEditing(false);
      return;
    }
    renameMut.mutate(next);
  };

  return (
    <TicketCard
      ticket={ticket}
      session={session}
      active={active}
      onClick={editing ? undefined : onSelect}
      onDoubleClick={(e) => {
        e.stopPropagation();
        setEditing(true);
      }}
      innerRef={setNodeRef}
      style={style}
      dragProps={editing ? undefined : { ...attributes, ...listeners }}
      interactive={!editing}
      titleSlot={
        editing ? (
          <input
            ref={inputRef}
            value={draft}
            onChange={(e) => setDraft(e.target.value)}
            onClick={(e) => e.stopPropagation()}
            onKeyDown={(e) => {
              if (e.key === "Enter") {
                e.preventDefault();
                submit();
              } else if (e.key === "Escape") {
                e.preventDefault();
                setEditing(false);
              }
            }}
            onBlur={submit}
            disabled={renameMut.isPending}
            className="min-w-0 flex-1 rounded bg-surface px-1 py-0.5 text-sm font-medium outline-none ring-1 ring-border focus:ring-accent-500"
          />
        ) : undefined
      }
    />
  );
}

export function TicketDragPreview({
  ticket,
  session,
  active,
}: {
  ticket: TicketType;
  session: Session | null;
  active: boolean;
}) {
  return (
    <TicketCard
      ticket={ticket}
      session={session}
      active={active}
      className="shadow-2xl ring-1 ring-border"
    />
  );
}
