import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { DndContext, DragEndEvent, PointerSensor, useSensor, useSensors } from "@dnd-kit/core";
import { useCallback, useState } from "react";
import { api } from "@/api/client";
import { queryKeys } from "@/api/keys";
import { useTerminalOrientation } from "@/hooks/useTerminalOrientation";
import { Column } from "./Column";
import { PtyTerminal } from "./PtyTerminal";
import { SessionPane } from "./SessionPane";

// Statuses where the container is up and the PTY broker can attach. We avoid
// mounting PtyTerminal during "starting" (set optimistically by the create/start
// flow) because the websocket would race the container spawn and the failed
// handshake leaves a stray "[disconnected]" line in the terminal.
const ATTACHABLE = new Set(["idle", "working", "awaiting_perm"]);

export function Board({ boardId }: { boardId: number }) {
  const qc = useQueryClient();
  const stateQ = useQuery({ queryKey: queryKeys.board(boardId), queryFn: () => api.boardState(boardId) });
  const [activeTicket, setActiveTicket] = useState<number | null>(null);
  const [terminalSlot, setTerminalSlot] = useState<HTMLDivElement | null>(null);
  const onTerminalSlot = useCallback((el: HTMLDivElement | null) => setTerminalSlot(el), []);
  const sensors = useSensors(useSensor(PointerSensor, { activationConstraint: { distance: 5 } }));
  const orientation = useTerminalOrientation();

  const moveMut = useMutation({
    mutationFn: (input: { id: number; column_id: number; position: number }) =>
      api.moveTicket(input.id, { column_id: input.column_id, position: input.position }),
    onSuccess: () => qc.invalidateQueries({ queryKey: queryKeys.board(boardId) }),
  });

  if (stateQ.isLoading) return <p className="p-4 text-sm text-zinc-400">Loading…</p>;
  if (!stateQ.data) return <p className="p-4 text-sm text-red-400">No data.</p>;

  const { board, columns, tickets, sessions, merge_config, sync_config } = stateQ.data;
  const sessionByTicket = new Map<number, (typeof sessions)[number]>(sessions.map((s) => [s.ticket_id, s]));

  function onDragEnd(e: DragEndEvent) {
    const ticketId = Number(e.active.id);
    const overId = e.over?.id;
    if (overId == null) return;
    const targetCol = Number(String(overId).replace(/^col-/, ""));
    if (Number.isNaN(targetCol)) return;
    const target = tickets.filter((t) => t.column_id === targetCol);
    moveMut.mutate({ id: ticketId, column_id: targetCol, position: target.length });
  }

  return (
    <div className={`flex h-full ${orientation === "horizontal" ? "flex-col" : "flex-row"}`}>
      <DndContext sensors={sensors} onDragEnd={onDragEnd}>
        <div data-board-area className="flex min-h-0 flex-1 gap-2 overflow-x-auto p-3">
          {columns.map((c) => (
            <Column
              key={c.id}
              column={c}
              tickets={tickets.filter((t) => t.column_id === c.id)}
              sessions={sessionByTicket}
              boardId={boardId}
              onSelect={setActiveTicket}
              activeTicket={activeTicket}
            />
          ))}
        </div>
      </DndContext>
      <SessionPane
        key={activeTicket ?? "none"}
        boardId={boardId}
        baseBranch={board.base_branch}
        mergeConfig={merge_config}
        syncConfig={sync_config}
        ticketId={activeTicket}
        session={activeTicket != null ? sessionByTicket.get(activeTicket) ?? null : null}
        onClose={() => setActiveTicket(null)}
        onTerminalSlot={onTerminalSlot}
        orientation={orientation}
      />
      {sessions
        .filter((s) => ATTACHABLE.has(s.status))
        .map((s) => (
          <PtyTerminal
            key={`${s.id}:${s.started_at ?? 0}`}
            sessionId={s.id}
            mountTarget={activeTicket === s.ticket_id ? terminalSlot : null}
          />
        ))}
    </div>
  );
}
