import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useState } from "react";
import { api, Session, subscribeBoard, Ticket } from "@/api/client";
import { queryKeys } from "@/api/keys";
import {
  activeTicketStore,
  sessionStore,
  ticketStore,
} from "@/store";
import { AppSettings } from "@/components/AppSettings";
import { ArchivedDrawer } from "@/components/ArchivedDrawer";
import { Board } from "@/components/Board";
import { BoardSettings } from "@/components/BoardSettings";
import { Button } from "@/components/Button";
import { CreateBoardForm } from "@/components/CreateBoardForm";
import { useAccent } from "@/hooks/useAccent";
import { useContrast } from "@/hooks/useContrast";
import { useThemeMode } from "@/hooks/useThemeMode";
import { CogIcon, MenuIcon } from "@/icons";
import { useShortcut } from "@/keys/useShortcut";
import { readActiveBoardId, writeActiveBoardId } from "@/storage";

export default function App() {
  useThemeMode();
  useContrast();
  useAccent();
  const qc = useQueryClient();
  const boardsQ = useQuery({ queryKey: queryKeys.boards, queryFn: api.listBoards });
  const [activeId, setActiveId] = useState<number | null>(null);
  const [streamStatus, setStreamStatus] = useState<"open" | "error" | "closed">("closed");
  const [showArchived, setShowArchived] = useState(false);
  const [showSettings, setShowSettings] = useState(false);
  const [showAppSettings, setShowAppSettings] = useState(false);

  const activeBoard = activeId != null ? boardsQ.data?.find((b) => b.id === activeId) ?? null : null;
  const noBoards = boardsQ.data?.length === 0;

  useEffect(() => {
    if (activeId == null && boardsQ.data && boardsQ.data.length > 0) {
      const remembered = readActiveBoardId();
      const fallback = boardsQ.data[0].id;
      setActiveId(remembered != null && boardsQ.data.some((b) => b.id === remembered) ? remembered : fallback);
    }
  }, [boardsQ.data, activeId]);

  useEffect(() => {
    if (activeId != null) writeActiveBoardId(activeId);
    // Selection lives outside the board; clear it when the board changes so a
    // ticket-id from board A doesn't leak into board B's pane.
    activeTicketStore.set(null);
  }, [activeId]);

  const cycleBoard = (delta: 1 | -1) => {
    const boards = boardsQ.data;
    if (!boards || boards.length < 2) return;
    const idx = boards.findIndex((b) => b.id === activeId);
    if (idx < 0) return;
    const next = (idx + delta + boards.length) % boards.length;
    setActiveId(boards[next].id);
  };
  useShortcut("board.next", () => cycleBoard(1));
  useShortcut("board.prev", () => cycleBoard(-1));

  useEffect(() => {
    if (activeId == null) return;
    const key = queryKeys.board(activeId);
    return subscribeBoard(activeId, {
      onEvent: (type, data) => {
        applyBoardEvent(activeId, type, data, () =>
          qc.invalidateQueries({ queryKey: key }),
        );
        if (type === "ticket_archived" || type === "ticket_unarchived" || type === "ticket_deleted") {
          qc.invalidateQueries({ queryKey: queryKeys.archived(activeId) });
        }
      },
      onStatus: setStreamStatus,
    });
  }, [activeId, qc]);

  return (
    <div className="flex h-full flex-col">
      <header className="flex items-center gap-4 border-b border-border px-3 py-2">
        <h1 className="text-lg font-semibold">Kanban</h1>
        <select
          className="cursor-pointer rounded bg-surface px-2 py-1 text-sm"
          value={activeId ?? ""}
          onChange={(e) => setActiveId(Number(e.target.value))}
        >
          <option value="">— select board —</option>
          {(boardsQ.data ?? []).map((b) => (
            <option key={b.id} value={b.id}>
              {b.name}
            </option>
          ))}
        </select>
        <div className="ml-auto flex items-center gap-2">
          <CreateBoardForm
            onCreated={(b) => {
              qc.invalidateQueries({ queryKey: queryKeys.boards });
              setActiveId(b.id);
            }}
          />
          {activeId != null && (
            <Button
              variant="neutral"
              size="sm"
              className="inline-flex h-7 items-center justify-center"
              onClick={() => setShowArchived(true)}
            >
              archived
            </Button>
          )}
          {activeBoard && (
            <Button
              variant="neutral"
              size="icon"
              onClick={() => setShowSettings(true)}
              aria-label="Board settings"
              title="Board settings"
            >
              <CogIcon />
            </Button>
          )}
          <Button
            variant="neutral"
            size="icon"
            onClick={() => setShowAppSettings(true)}
            aria-label="App settings"
            title="App settings"
          >
            <MenuIcon />
          </Button>
        </div>
      </header>
      {activeId != null && streamStatus === "error" && (
        <div className="border-b border-amber-700 bg-amber-950/60 px-4 py-1 text-xs text-amber-200">
          Live updates disconnected — reconnecting…
        </div>
      )}
      <main className="min-h-0 flex-1 overflow-hidden">
        {activeId != null ? (
          <Board boardId={activeId} />
        ) : noBoards ? (
          <div className="flex h-full items-center justify-center p-4 text-sm text-fg-muted">
            No board selected.
          </div>
        ) : null}
      </main>
      {activeId != null && showArchived && (
        <ArchivedDrawer open boardId={activeId} onClose={() => setShowArchived(false)} />
      )}
      {activeBoard && showSettings && (
        <BoardSettings
          open
          board={activeBoard}
          onClose={() => setShowSettings(false)}
          onDeleted={() => {
            setShowSettings(false);
            setActiveId(null);
          }}
        />
      )}
      {showAppSettings && <AppSettings open onClose={() => setShowAppSettings(false)} />}
    </div>
  );
}

// Routes an SSE event to the right cache. Per-id content updates flow
// straight to the entity stores (no cascade). Structural changes — anything
// that adds, removes, or reorders ticket-ids in a column — invalidate the
// boardStructure query so it refetches and rebuilds the index.
function applyBoardEvent(
  boardId: number,
  type: string,
  data: unknown,
  invalidateStructure: () => void,
): void {
  switch (type) {
    case "ticket_updated": {
      const t = data as Ticket | null;
      if (t) ticketStore.set(t.id, t);
      return;
    }
    case "ticket_created":
    case "ticket_moved":
    case "ticket_unarchived": {
      const t = data as Ticket | null;
      if (t) ticketStore.set(t.id, t);
      invalidateStructure();
      return;
    }
    case "ticket_archived":
    case "ticket_deleted": {
      const t = data as Ticket | null;
      if (t && activeTicketStore.get() === t.id && t.board_id === boardId) {
        activeTicketStore.set(null);
      }
      invalidateStructure();
      return;
    }
    case "session_updated": {
      const s = data as Session | null;
      if (!s) return;
      const prev = sessionStore.get(s.id);
      sessionStore.set(s.id, s);
      // New session: ticket → session map needs rebuild.
      if (!prev) invalidateStructure();
      return;
    }
    case "ready":
    default:
      return;
  }
}
