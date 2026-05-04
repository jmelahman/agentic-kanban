import { useDraggable } from "@dnd-kit/core";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useEffect, useRef, useState } from "react";
import { api, Session, Ticket as TicketType } from "@/api/client";
import { queryKeys } from "@/api/keys";

const STATUS_COLOR: Record<string, string> = {
  stopped: "text-zinc-500",
  starting: "text-amber-400",
  stopping: "text-amber-400",
  idle: "text-emerald-400",
  working: "text-sky-400",
  awaiting_perm: "text-yellow-400",
  error: "text-red-400",
};

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

  const { attributes, listeners, setNodeRef, transform, isDragging } = useDraggable({
    id: ticket.id,
    disabled: editing,
  });
  const style: React.CSSProperties = {
    transform: transform ? `translate3d(${transform.x}px, ${transform.y}px, 0)` : undefined,
    opacity: isDragging ? 0.5 : 1,
  };
  const status = session?.status ?? "stopped";

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
      // Defer focus until the input has been rendered.
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
    <div
      ref={setNodeRef}
      {...(editing ? {} : attributes)}
      {...(editing ? {} : listeners)}
      style={style}
      onClick={editing ? undefined : onSelect}
      onDoubleClick={(e) => {
        e.stopPropagation();
        setEditing(true);
      }}
      data-ticket-card="true"
      className={`rounded bg-zinc-800 p-2 text-sm transition-colors duration-150 ${editing ? "" : "cursor-pointer hover:bg-zinc-700"} ${active ? "ring-2 ring-red-500" : ""}`}
    >
      <div className="flex items-center justify-between gap-2">
        {editing ? (
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
            className="min-w-0 flex-1 rounded bg-zinc-900 px-1 py-0.5 text-sm font-medium outline-none ring-1 ring-zinc-700 focus:ring-red-500"
          />
        ) : (
          <span className="font-medium">{ticket.title}</span>
        )}
        <span className={`text-xs ${STATUS_COLOR[status] ?? "text-zinc-500"}`}>{status}</span>
      </div>
      {ticket.body && <p className="mt-1 text-xs text-zinc-400 line-clamp-2">{ticket.body}</p>}
    </div>
  );
}
