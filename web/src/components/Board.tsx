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
import { useShortcut } from "@/keys/useShortcut";
import {
  activeTicketStore,
  addTicketRequestStore,
  BoardStructure,
  fetchBoardStructure,
  ScalarStore,
  ticketStore,
  useIsActiveTicket,
  useScalarStore,
  useSession,
  useTicket,
} from "@/store";
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
  structure: BoardStructure,
): number | null {
  const { columns, ticketIdsByColumn } = structure;
  if (columns.length === 0) return activeId;
  if (activeId == null) {
    for (const c of columns) {
      const list = ticketIdsByColumn[c.id] ?? [];
      if (list.length > 0) return list[0];
    }
    return null;
  }
  const colIdx = columns.findIndex((c) =>
    (ticketIdsByColumn[c.id] ?? []).includes(activeId),
  );
  if (colIdx === -1) return activeId;
  const list = ticketIdsByColumn[columns[colIdx].id] ?? [];
  const pos = list.indexOf(activeId);
  if (direction === "up" || direction === "down") {
    const next = list[pos + (direction === "down" ? 1 : -1)];
    return next ?? activeId;
  }
  const delta = direction === "right" ? 1 : -1;
  for (let i = colIdx + delta; i >= 0 && i < columns.length; i += delta) {
    const candidates = ticketIdsByColumn[columns[i].id] ?? [];
    if (candidates.length === 0) continue;
    const targetIdx = Math.min(pos, candidates.length - 1);
    return candidates[targetIdx];
  }
  return activeId;
}

function moveTicketIdInStructure(
  structure: BoardStructure,
  movedId: number,
  targetCol: number,
  insertIndex: number,
): BoardStructure {
  const ticketIdsByColumn: Record<number, number[]> = {};
  for (const [colKey, ids] of Object.entries(structure.ticketIdsByColumn)) {
    ticketIdsByColumn[Number(colKey)] = ids.filter((id) => id !== movedId);
  }
  const target = ticketIdsByColumn[targetCol] ?? [];
  target.splice(insertIndex, 0, movedId);
  ticketIdsByColumn[targetCol] = target;
  return { ...structure, ticketIdsByColumn };
}

export function Board({ boardId }: { boardId: number }) {
  const qc = useQueryClient();
  const stateQ = useQuery({
    queryKey: queryKeys.board(boardId),
    queryFn: () => fetchBoardStructure(boardId),
  });
  const [draggingId, setDraggingId] = useState<number | null>(null);
  // Slots are SessionPane-owned DOM nodes that PtyTerminals portal into.
  // Held in ScalarStores rather than `useState` so SessionPane mounting /
  // unmounting its slot div doesn't re-render Board (and its 4 columns × N
  // tickets). Only the few `SessionTerminals` instances that subscribe react.
  const [agentSlotStore] = useState(() => new ScalarStore<HTMLDivElement | null>(null));
  const [shellSlotStore] = useState(() => new ScalarStore<HTMLDivElement | null>(null));
  const onAgentSlot = useCallback(
    (el: HTMLDivElement | null) => agentSlotStore.set(el),
    [agentSlotStore],
  );
  const onShellSlot = useCallback(
    (el: HTMLDivElement | null) => shellSlotStore.set(el),
    [shellSlotStore],
  );
  const sensors = useSensors(useSensor(PointerSensor, { activationConstraint: { distance: 5 } }));
  const orientation = useTerminalOrientation();

  const moveMut = useMutation({
    mutationFn: (input: { id: number; column_id: number; position: number }) =>
      api.moveTicket(input.id, { column_id: input.column_id, position: input.position }),
    onSettled: () => qc.invalidateQueries({ queryKey: queryKeys.board(boardId) }),
  });

  const structure = stateQ.data;
  const moveSelection = useCallback(
    (direction: Direction) => {
      if (!structure) return;
      const next = nextTicketId(direction, activeTicketStore.get(), structure);
      if (next != null) activeTicketStore.set(next);
    },
    [structure],
  );
  useShortcut("ticket.prev", () => moveSelection("up"));
  useShortcut("ticket.next", () => moveSelection("down"));
  useShortcut("column.prev", () => moveSelection("left"));
  useShortcut("column.next", () => moveSelection("right"));
  useShortcut("ticket.create", () => {
    if (!structure) return;
    const { columns, ticketIdsByColumn } = structure;
    if (columns.length === 0) return;
    const activeId = activeTicketStore.get();
    const activeColIdx =
      activeId == null
        ? -1
        : columns.findIndex((c) =>
            (ticketIdsByColumn[c.id] ?? []).includes(activeId),
          );
    const target = columns[activeColIdx >= 0 ? activeColIdx : 0];
    addTicketRequestStore.set(target.id);
  });

  if (stateQ.isLoading) return <p className="p-4 text-sm text-fg-muted">Loading…</p>;
  if (!structure) return <p className="p-4 text-sm text-danger">No data.</p>;

  const { board, columns, ticketIdsByColumn, sessionIdByTicket, merge_config, sync_config } =
    structure;

  function onDragStart(e: DragStartEvent) {
    setDraggingId(Number(e.active.id));
  }

  function onDragEnd(e: DragEndEvent) {
    setDraggingId(null);
    const ticketId = Number(e.active.id);
    const overId = e.over?.id;
    if (overId == null) return;
    const moved = ticketStore.get(ticketId);
    if (!moved) return;

    // over.id is either `col-N` (column droppable) or a ticket id (sortable item).
    let targetCol: number;
    let insertIndex: number;
    if (typeof overId === "string" && overId.startsWith("col-")) {
      targetCol = Number(overId.slice(4));
      if (Number.isNaN(targetCol)) return;
      const list = ticketIdsByColumn[targetCol] ?? [];
      insertIndex = list.filter((id) => id !== ticketId).length;
    } else {
      const overTicketId = Number(overId);
      const overTicket = ticketStore.get(overTicketId);
      if (!overTicket) return;
      targetCol = overTicket.column_id;
      const targetList = (ticketIdsByColumn[targetCol] ?? []).filter(
        (id) => id !== ticketId,
      );
      const idx = targetList.indexOf(overTicketId);
      if (idx < 0) {
        insertIndex = targetList.length;
      } else if (
        moved.column_id === targetCol &&
        moved.position < overTicket.position
      ) {
        // Dragging downward within the same column: drop after the over ticket.
        insertIndex = idx + 1;
      } else {
        insertIndex = idx;
      }
    }

    if (moved.column_id === targetCol) {
      const sourceList = ticketIdsByColumn[targetCol] ?? [];
      if (sourceList.indexOf(ticketId) === insertIndex) return;
    }

    qc.setQueryData<BoardStructure>(queryKeys.board(boardId), (old) =>
      old ? moveTicketIdInStructure(old, ticketId, targetCol, insertIndex) : old,
    );
    moveMut.mutate({ id: ticketId, column_id: targetCol, position: insertIndex });
  }

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
              ticketIds={ticketIdsByColumn[c.id] ?? []}
              sessionIdByTicket={sessionIdByTicket}
              boardId={boardId}
            />
          ))}
        </div>
        <DragOverlay>
          {draggingId != null ? (
            <DraggingPreview
              id={draggingId}
              sessionId={sessionIdByTicket[draggingId] ?? null}
            />
          ) : null}
        </DragOverlay>
      </DndContext>
      <SessionPane
        boardId={boardId}
        baseBranch={board.base_branch}
        mergeConfig={merge_config}
        syncConfig={sync_config}
        sessionIdByTicket={sessionIdByTicket}
        onAgentSlot={onAgentSlot}
        onShellSlot={onShellSlot}
        orientation={orientation}
      />
      {Object.entries(sessionIdByTicket).map(([tIdStr, sId]) => (
        <SessionTerminals
          key={sId}
          ticketId={Number(tIdStr)}
          sessionId={sId}
          agentSlotStore={agentSlotStore}
          shellSlotStore={shellSlotStore}
        />
      ))}
    </div>
  );
}

// Subscribes to one session's status. Status transitions (e.g. starting → idle)
// notify only this instance, so the PtyTerminal can mount/unmount without the
// rest of the board re-rendering.
function SessionTerminals({
  ticketId,
  sessionId,
  agentSlotStore,
  shellSlotStore,
}: {
  ticketId: number;
  sessionId: number;
  agentSlotStore: ScalarStore<HTMLDivElement | null>;
  shellSlotStore: ScalarStore<HTMLDivElement | null>;
}) {
  const session = useSession(sessionId);
  const isActive = useIsActiveTicket(ticketId);
  const agentSlot = useScalarStore(agentSlotStore);
  const shellSlot = useScalarStore(shellSlotStore);
  // The shell tab is lazy: we only spawn the shell PTY once the user has
  // opened the shell tab at least once for *this* session (slot ref fires
  // non-null while we're the active ticket).
  const [shellEverOpened, setShellEverOpened] = useState(false);
  useEffect(() => {
    if (isActive && shellSlot) setShellEverOpened(true);
  }, [isActive, shellSlot]);
  if (!session || !ATTACHABLE.has(session.status)) return null;
  return (
    <>
      <PtyTerminal
        key={`agent:${session.started_at ?? 0}`}
        sessionId={session.id}
        kind="agent"
        mountTarget={isActive ? agentSlot : null}
      />
      {shellEverOpened && (
        <PtyTerminal
          key={`shell:${session.started_at ?? 0}`}
          sessionId={session.id}
          kind="shell"
          mountTarget={isActive ? shellSlot : null}
        />
      )}
    </>
  );
}

function DraggingPreview({ id, sessionId }: { id: number; sessionId: number | null }) {
  const ticket = useTicket(id);
  const session = useSession(sessionId);
  if (!ticket) return null;
  return (
    <TicketDragPreview
      ticket={ticket}
      session={session ?? null}
      active={activeTicketStore.get() === id}
    />
  );
}
