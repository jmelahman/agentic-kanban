import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useDroppable } from "@dnd-kit/core";
import { useState } from "react";
import { api, Column as ColumnType, Session, Ticket as TicketType } from "@/api/client";
import { queryKeys } from "@/api/keys";
import { Button } from "./Button";
import { Ticket } from "./Ticket";

export function Column(props: {
  column: ColumnType;
  tickets: TicketType[];
  sessions: Map<number, Session>;
  boardId: number;
  activeTicket: number | null;
  onSelect: (id: number) => void;
}) {
  const qc = useQueryClient();
  const { setNodeRef, isOver } = useDroppable({ id: `col-${props.column.id}` });
  const [adding, setAdding] = useState(false);
  const [title, setTitle] = useState("");

  const createMut = useMutation({
    mutationFn: () => api.createTicket(props.boardId, { column_id: props.column.id, title }),
    onSuccess: () => {
      setTitle("");
      setAdding(false);
      qc.invalidateQueries({ queryKey: queryKeys.board(props.boardId) });
    },
  });

  return (
    <div
      ref={setNodeRef}
      className={`flex h-full min-w-72 flex-1 flex-col gap-2 overflow-hidden rounded border border-border bg-surface p-2 ${isOver ? "ring-2 ring-accent-600" : ""}`}
    >
      <div className="flex items-center justify-between">
        <h2 className="text-sm font-semibold uppercase tracking-wide text-fg">{props.column.name}</h2>
        <span className="text-xs text-fg-muted">{props.tickets.length}</span>
      </div>
      <div className="flex min-h-0 flex-1 flex-col gap-2 overflow-y-auto">
        {props.tickets
          .slice()
          .sort((a, b) => a.position - b.position)
          .map((t) => (
            <Ticket
              key={t.id}
              ticket={t}
              session={props.sessions.get(t.id) ?? null}
              active={props.activeTicket === t.id}
              onSelect={() => props.onSelect(t.id)}
            />
          ))}
      </div>
      {adding ? (
        <form
          onSubmit={(e) => {
            e.preventDefault();
            if (title.trim() && !createMut.isPending) createMut.mutate();
          }}
          className="flex flex-col gap-1"
        >
          <input
            autoFocus
            className="rounded bg-surface-2 px-2 py-1 text-sm"
            value={title}
            onChange={(e) => setTitle(e.target.value)}
            placeholder="ticket title"
            disabled={createMut.isPending}
          />
          <div className="flex gap-2 text-xs">
            <Button
              variant="primary"
              type="submit"
              pending={createMut.isPending}
              idleLabel="add"
              pendingLabel="adding…"
            />
            <Button
              variant="ghost"
              type="button"
              onClick={() => setAdding(false)}
              disabled={createMut.isPending}
            >
              cancel
            </Button>
          </div>
        </form>
      ) : (
        <Button variant="dashed" className="text-xs" onClick={() => setAdding(true)}>
          + add ticket
        </Button>
      )}
    </div>
  );
}
