import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  DndContext,
  DragEndEvent,
  DragOverlay,
  DragStartEvent,
  PointerSensor,
  useSensor,
  useSensors,
} from "@dnd-kit/core";
import { useCallback, useEffect, useState } from "react";
import { api } from "@/api/client";
import { queryKeys } from "@/api/keys";
import { useTerminalOrientation } from "@/hooks/useTerminalOrientation";
import { Column } from "./Column";
import { PtyTerminal } from "./PtyTerminal";
import { SessionPane } from "./SessionPane";
import { TicketDragPreview } from "./Ticket";

// Statuses where the container is up and the PTY broker can attach. We avoid
// mounting PtyTerminal during "starting" (set optimistically by the create/start
// flow) because the websocket would race the container spawn and the failed
// handshake leaves a stray "[disconnected]" line in the terminal.
const ATTACHABLE = new Set(["idle", "working", "awaiting_perm"]);

export function Board({ boardId }: { boardId: number }) {
  const qc = useQueryClient();
  const stateQ = useQuery({ queryKey: queryKeys.board(boardId), queryFn: () => api.boardState(boardId) });
  const [activeTicket, setActiveTicket] = useState<number | null>(null);
  const [draggingId, setDraggingId] = useState<number | null>(null);
  const [agentSlot, setAgentSlot] = useState<HTMLDivElement | null>(null);
  const onAgentSlot = useCallback((el: HTMLDivElement | null) => setAgentSlot(el), []);
  const [shellSlot, setShellSlot] = useState<HTMLDivElement | null>(null);
  const onShellSlot = useCallback((el: HTMLDivElement | null) => setShellSlot(el), []);
  const [shellOpened, setShellOpened] = useState<Set<number>>(() => new Set());
  const sensors = useSensors(useSensor(PointerSensor, { activationConstraint: { distance: 5 } }));
  const orientation = useTerminalOrientation();

  const moveMut = useMutation({
    mutationFn: (input: { id: number; column_id: number; position: number }) =>
      api.moveTicket(input.id, { column_id: input.column_id, position: input.position }),
    onSuccess: () => qc.invalidateQueries({ queryKey: queryKeys.board(boardId) }),
  });

  const sessions = stateQ.data?.sessions ?? [];
  const sessionByTicket = new Map<number, (typeof sessions)[number]>(sessions.map((s) => [s.ticket_id, s]));
  const activeSession = activeTicket != null ? sessionByTicket.get(activeTicket) ?? null : null;
  const activeSessionId = activeSession?.id ?? null;

  // The shell tab is lazy: we only spawn the shell PTY once the user has opened
  // the shell tab at least once for that session (the slot ref fires non-null).
  useEffect(() => {
    if (shellSlot && activeSessionId != null) {
      setShellOpened((prev) => {
        if (prev.has(activeSessionId)) return prev;
        const next = new Set(prev);
        next.add(activeSessionId);
        return next;
      });
    }
  }, [shellSlot, activeSessionId]);

  if (stateQ.isLoading) return <p className="p-4 text-sm text-fg-muted">Loading…</p>;
  if (!stateQ.data) return <p className="p-4 text-sm text-danger">No data.</p>;

  const { board, columns, tickets, merge_config, sync_config } = stateQ.data;

  function onDragStart(e: DragStartEvent) {
    setDraggingId(Number(e.active.id));
  }

  function onDragEnd(e: DragEndEvent) {
    setDraggingId(null);
    const ticketId = Number(e.active.id);
    const overId = e.over?.id;
    if (overId == null) return;
    const targetCol = Number(String(overId).replace(/^col-/, ""));
    if (Number.isNaN(targetCol)) return;
    const target = tickets.filter((t) => t.column_id === targetCol);
    moveMut.mutate({ id: ticketId, column_id: targetCol, position: target.length });
  }

  const draggingTicket = draggingId != null ? tickets.find((t) => t.id === draggingId) ?? null : null;

  return (
    <div className={`flex h-full ${orientation === "horizontal" ? "flex-col" : "flex-row"}`}>
      <DndContext
        sensors={sensors}
        onDragStart={onDragStart}
        onDragEnd={onDragEnd}
        onDragCancel={() => setDraggingId(null)}
      >
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
        <DragOverlay>
          {draggingTicket ? (
            <TicketDragPreview
              ticket={draggingTicket}
              session={sessionByTicket.get(draggingTicket.id) ?? null}
              active={activeTicket === draggingTicket.id}
            />
          ) : null}
        </DragOverlay>
      </DndContext>
      <SessionPane
        key={activeTicket ?? "none"}
        boardId={boardId}
        baseBranch={board.base_branch}
        mergeConfig={merge_config}
        syncConfig={sync_config}
        ticketId={activeTicket}
        session={activeSession}
        onClose={() => setActiveTicket(null)}
        onAgentSlot={onAgentSlot}
        onShellSlot={onShellSlot}
        orientation={orientation}
      />
      {sessions
        .filter((s) => ATTACHABLE.has(s.status))
        .flatMap((s) => {
          const isActive = activeTicket === s.ticket_id;
          const elements = [
            <PtyTerminal
              key={`${s.id}:agent:${s.started_at ?? 0}`}
              sessionId={s.id}
              kind="agent"
              mountTarget={isActive ? agentSlot : null}
            />,
          ];
          if (shellOpened.has(s.id)) {
            elements.push(
              <PtyTerminal
                key={`${s.id}:shell:${s.started_at ?? 0}`}
                sessionId={s.id}
                kind="shell"
                mountTarget={isActive ? shellSlot : null}
              />,
            );
          }
          return elements;
        })}
    </div>
  );
}
