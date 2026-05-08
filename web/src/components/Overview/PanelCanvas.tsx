import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useCallback, useEffect, useState } from "react";
import { Rnd } from "react-rnd";
import { api } from "@/api/client";
import { queryKeys } from "@/api/keys";
import { fetchBoardStructure } from "@/store";
import type { BoardStructure } from "@/store";
import { SessionView } from "@/components/SessionView";
import { loadPanels, writePanels, type PersistedPanel } from "./storage";

export type PanelCanvasHandle = {
  open: (boardId: number, ticketId: number) => void;
};

const DEFAULT_W = 640;
const DEFAULT_H = 480;
const CASCADE_STEP = 28;

// Free-form workspace of draggable, resizable session panels. Each panel
// renders a SessionView for one ticket; multiple may be open at once.
// Layout is persisted to localStorage and re-validated against the live
// ticket set whenever a board structure refetches (covers archive/delete
// in another tab).
export function PanelCanvas({
  registerHandle,
}: {
  registerHandle: (handle: PanelCanvasHandle | null) => void;
}) {
  const [panels, setPanels] = useState<PersistedPanel[]>(loadPanels);
  const [focusedTicketId, setFocusedTicketId] = useState<number | null>(null);
  const qc = useQueryClient();

  // Persist on every layout change.
  useEffect(() => {
    writePanels(panels);
  }, [panels]);

  // Filter panels whose ticket no longer exists on its board. Re-runs when
  // any board query is added, removed, or invalidated; cheap because we
  // only touch the panels array. Defers the setState via queueMicrotask so
  // we never call it synchronously from a child component's render — Panel's
  // own useQuery can fire cache notifications mid-render.
  useEffect(() => {
    const recheck = () => {
      setPanels((prev) => {
        const next = prev.filter((p) => isLiveTicket(qc, p.boardId, p.ticketId));
        if (next.length === prev.length) return prev;
        return next;
      });
    };
    recheck();
    return qc.getQueryCache().subscribe(() => {
      queueMicrotask(recheck);
    });
  }, [qc]);

  const open = useCallback(
    (boardId: number, ticketId: number) => {
      setPanels((prev) => {
        const idx = prev.findIndex((p) => p.ticketId === ticketId);
        if (idx >= 0) {
          // Bring to front: splice to end of array.
          const next = prev.slice();
          const [moved] = next.splice(idx, 1);
          next.push(moved);
          return next;
        }
        const last = prev[prev.length - 1];
        const x = last ? Math.max(0, last.x + CASCADE_STEP) : 24;
        const y = last ? Math.max(0, last.y + CASCADE_STEP) : 24;
        return [
          ...prev,
          { ticketId, boardId, x, y, width: DEFAULT_W, height: DEFAULT_H },
        ];
      });
      setFocusedTicketId(ticketId);
    },
    [],
  );

  useEffect(() => {
    registerHandle({ open });
    return () => registerHandle(null);
  }, [open, registerHandle]);

  const close = useCallback((ticketId: number) => {
    setPanels((prev) => prev.filter((p) => p.ticketId !== ticketId));
    setFocusedTicketId((cur) => (cur === ticketId ? null : cur));
  }, []);

  const update = useCallback(
    (ticketId: number, patch: Partial<PersistedPanel>) => {
      setPanels((prev) =>
        prev.map((p) => (p.ticketId === ticketId ? { ...p, ...patch } : p)),
      );
    },
    [],
  );

  const bringToFront = useCallback((ticketId: number) => {
    setPanels((prev) => {
      const idx = prev.findIndex((p) => p.ticketId === ticketId);
      if (idx < 0 || idx === prev.length - 1) return prev;
      const next = prev.slice();
      const [moved] = next.splice(idx, 1);
      next.push(moved);
      return next;
    });
  }, []);

  return (
    <div className="relative h-full w-full overflow-hidden bg-surface">
      {panels.length === 0 && (
        <div className="absolute inset-0 flex items-center justify-center text-sm text-fg-muted">
          Click a ticket on the left to open a session panel.
        </div>
      )}
      {panels.map((p) => (
        <Panel
          key={p.ticketId}
          panel={p}
          focused={p.ticketId === focusedTicketId}
          onClose={() => close(p.ticketId)}
          onUpdate={(patch) => update(p.ticketId, patch)}
          onBringToFront={() => bringToFront(p.ticketId)}
          onFocus={() => setFocusedTicketId(p.ticketId)}
        />
      ))}
    </div>
  );
}

function Panel({
  panel,
  focused,
  onClose,
  onUpdate,
  onBringToFront,
  onFocus,
}: {
  panel: PersistedPanel;
  focused: boolean;
  onClose: () => void;
  onUpdate: (patch: Partial<PersistedPanel>) => void;
  onBringToFront: () => void;
  onFocus: () => void;
}) {
  // Each panel needs its board's config (mergeConfig, syncConfig, baseBranch,
  // sessionIdByTicket). Same query key as the tree's BoardNode so requests
  // dedupe; this just reads from the shared cache.
  const boardQ = useQuery({
    queryKey: queryKeys.board(panel.boardId),
    queryFn: () => fetchBoardStructure(panel.boardId),
  });
  const structure = boardQ.data;

  if (!structure) {
    return (
      <Rnd
        position={{ x: panel.x, y: panel.y }}
        size={{ width: panel.width, height: panel.height }}
        bounds="parent"
        minWidth={320}
        minHeight={240}
        dragHandleClassName="panel-drag-handle"
        onDragStop={(_e, d) => onUpdate({ x: d.x, y: d.y })}
        onResizeStop={(_e, _dir, ref, _delta, pos) =>
          onUpdate({
            width: ref.offsetWidth,
            height: ref.offsetHeight,
            x: pos.x,
            y: pos.y,
          })
        }
        className="rounded border border-border bg-bg shadow-lg"
      >
        <div className="flex h-full items-center justify-center text-sm text-fg-muted">
          Loading board…
        </div>
      </Rnd>
    );
  }

  return (
    <Rnd
      position={{ x: panel.x, y: panel.y }}
      size={{ width: panel.width, height: panel.height }}
      bounds="parent"
      minWidth={320}
      minHeight={240}
      dragHandleClassName="panel-drag-handle"
      onMouseDown={onBringToFront}
      onDragStop={(_e, d) => onUpdate({ x: d.x, y: d.y })}
      onResizeStop={(_e, _dir, ref, _delta, pos) =>
        onUpdate({
          width: ref.offsetWidth,
          height: ref.offsetHeight,
          x: pos.x,
          y: pos.y,
        })
      }
      className={`overflow-hidden rounded border bg-bg shadow-lg ${focused ? "border-accent-500" : "border-border"}`}
    >
      <div
        tabIndex={0}
        onFocus={onFocus}
        onMouseDown={onFocus}
        className="flex h-full flex-col outline-none"
      >
        <div className="panel-drag-handle flex h-5 cursor-move items-center bg-surface-2 px-2 text-[10px] uppercase tracking-wide text-fg-muted">
          drag
        </div>
        <div className="min-h-0 flex-1">
          <SessionView
            ticketId={panel.ticketId}
            boardId={panel.boardId}
            baseBranch={structure.board.base_branch}
            mergeConfig={structure.merge_config}
            syncConfig={structure.sync_config}
            sessionIdByTicket={structure.sessionIdByTicket}
            onClose={onClose}
            shortcutsEnabled={focused}
          />
        </div>
      </div>
    </Rnd>
  );
}

function isLiveTicket(
  qc: ReturnType<typeof useQueryClient>,
  boardId: number,
  ticketId: number,
): boolean {
  const data = qc.getQueryData<BoardStructure>(queryKeys.board(boardId));
  // If the board hasn't loaded yet, give it the benefit of the doubt — we
  // don't want to drop a panel just because its board structure hasn't
  // arrived from the network.
  if (!data) return true;
  for (const ids of Object.values(data.ticketIdsByColumn)) {
    if (ids.includes(ticketId)) return true;
  }
  return false;
}
