import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { BoardState, Ticket as TicketType } from "@/api/client";
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
import { useShortcut } from "@/keys/useShortcut";
import { Column } from "./Column";
import { PtyTerminal } from "./PtyTerminal";
import { SessionPane } from "./SessionPane";
import { TicketDragPreview } from "./Ticket";

// Statuses where the container is up and the PTY broker can attach. We avoid
// mounting PtyTerminal during "starting" (set optimistically by the create/start
// flow) because the websocket would race the container spawn and the failed
// handshake leaves a stray "[disconnected]" line in the terminal.
const ATTACHABLE = new Set(["idle", "working", "awaiting_perm"]);

type Direction = "up" | "down" | "left" | "right";

function nextTicketId(
  direction: Direction,
  activeId: number | null,
  columns: { id: number }[],
  tickets: TicketType[],
): number | null {
  if (columns.length === 0) return activeId;
  const byCol = new Map<number, TicketType[]>();
  for (const c of columns) {
    byCol.set(
      c.id,
      tickets.filter((t) => t.column_id === c.id).sort((a, b) => a.position - b.position),
    );
  }
  if (activeId == null) {
    for (const c of columns) {
      const list = byCol.get(c.id)!;
      if (list.length > 0) return list[0].id;
    }
    return null;
  }
  const active = tickets.find((t) => t.id === activeId);
  if (!active) return activeId;
  const colIdx = columns.findIndex((c) => c.id === active.column_id);
  if (colIdx === -1) return activeId;
  const list = byCol.get(active.column_id)!;
  const pos = list.findIndex((t) => t.id === activeId);
  if (direction === "up" || direction === "down") {
    const next = list[pos + (direction === "down" ? 1 : -1)];
    return next?.id ?? activeId;
  }
  const delta = direction === "right" ? 1 : -1;
  for (let i = colIdx + delta; i >= 0 && i < columns.length; i += delta) {
    const candidates = byCol.get(columns[i].id)!;
    if (candidates.length === 0) continue;
    const targetIdx = Math.min(pos, candidates.length - 1);
    return candidates[targetIdx].id;
  }
  return activeId;
}

function reorderTickets(
  tickets: TicketType[],
  movedId: number,
  targetCol: number,
  insertIndex: number,
): TicketType[] {
  const moved = tickets.find((t) => t.id === movedId);
  if (!moved) return tickets;
  const affected = new Set([moved.column_id, targetCol]);
  const lists = new Map<number, TicketType[]>();
  for (const colId of affected) {
    lists.set(
      colId,
      tickets
        .filter((t) => t.column_id === colId && t.id !== movedId)
        .sort((a, b) => a.position - b.position),
    );
  }
  const targetList = lists.get(targetCol)!;
  targetList.splice(insertIndex, 0, { ...moved, column_id: targetCol });
  return tickets.map((t) => {
    if (t.id === movedId) {
      return { ...t, column_id: targetCol, position: targetList.findIndex((x) => x.id === movedId) };
    }
    if (affected.has(t.column_id)) {
      const idx = lists.get(t.column_id)!.findIndex((x) => x.id === t.id);
      if (idx >= 0) return { ...t, position: idx };
    }
    return t;
  });
}

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
    onSettled: () => qc.invalidateQueries({ queryKey: queryKeys.board(boardId) }),
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

  const navColumns = stateQ.data?.columns ?? [];
  const navTickets = stateQ.data?.tickets ?? [];
  const moveSelection = useCallback(
    (direction: Direction) => {
      const next = nextTicketId(direction, activeTicket, navColumns, navTickets);
      if (next != null) setActiveTicket(next);
    },
    [activeTicket, navColumns, navTickets],
  );
  useShortcut("ticket.prev", () => moveSelection("up"));
  useShortcut("ticket.next", () => moveSelection("down"));
  useShortcut("column.prev", () => moveSelection("left"));
  useShortcut("column.next", () => moveSelection("right"));

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
    const moved = tickets.find((t) => t.id === ticketId);
    if (!moved) return;

    // over.id is either `col-N` (column droppable) or a ticket id (sortable item).
    let targetCol: number;
    let insertIndex: number;
    if (typeof overId === "string" && overId.startsWith("col-")) {
      targetCol = Number(overId.slice(4));
      if (Number.isNaN(targetCol)) return;
      insertIndex = tickets.filter((t) => t.column_id === targetCol && t.id !== ticketId).length;
    } else {
      const overTicket = tickets.find((t) => t.id === Number(overId));
      if (!overTicket) return;
      targetCol = overTicket.column_id;
      const targetList = tickets
        .filter((t) => t.column_id === targetCol && t.id !== ticketId)
        .sort((a, b) => a.position - b.position);
      const idx = targetList.findIndex((t) => t.id === overTicket.id);
      if (idx < 0) {
        insertIndex = targetList.length;
      } else if (moved.column_id === targetCol && moved.position < overTicket.position) {
        // Dragging downward within the same column: drop after the over ticket.
        insertIndex = idx + 1;
      } else {
        insertIndex = idx;
      }
    }

    if (moved.column_id === targetCol) {
      const sourceList = tickets
        .filter((t) => t.column_id === targetCol)
        .sort((a, b) => a.position - b.position);
      if (sourceList.findIndex((t) => t.id === ticketId) === insertIndex) return;
    }

    qc.setQueryData<BoardState>(queryKeys.board(boardId), (old) =>
      old ? { ...old, tickets: reorderTickets(old.tickets, ticketId, targetCol, insertIndex) } : old,
    );
    moveMut.mutate({ id: ticketId, column_id: targetCol, position: insertIndex });
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
