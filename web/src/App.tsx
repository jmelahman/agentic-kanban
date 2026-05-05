import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useState } from "react";
import { api, BoardState, Session, subscribeBoard, Ticket } from "@/api/client";
import { queryKeys } from "@/api/keys";
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
  }, [activeId]);

  const boards = boardsQ.data;
  const canCycleBoards = (boards?.length ?? 0) > 1;
  const cycleBoard = (delta: 1 | -1) => {
    if (!boards || boards.length < 2) return;
    const idx = boards.findIndex((b) => b.id === activeId);
    const start = idx === -1 ? 0 : idx;
    const next = boards[(start + delta + boards.length) % boards.length];
    setActiveId(next.id);
  };
  useShortcut("board.next", () => cycleBoard(1), { enabled: canCycleBoards });
  useShortcut("board.prev", () => cycleBoard(-1), { enabled: canCycleBoards });

  useEffect(() => {
    if (activeId == null) return;
    const key = queryKeys.board(activeId);
    return subscribeBoard(activeId, {
      onEvent: (type, data) => {
        const patched = applyBoardEvent(qc.getQueryData<BoardState>(key), type, data);
        if (patched === undefined) {
          qc.invalidateQueries({ queryKey: key });
        } else if (patched !== null) {
          qc.setQueryData(key, patched);
        }
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
          className="rounded bg-surface px-2 py-1 text-sm"
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

// Patches the cached BoardState with an SSE event payload. Returns the new
// state, null to indicate "no change" (event for a different board / no cache
// yet), or undefined to fall back to refetching.
function applyBoardEvent(
  prev: BoardState | undefined,
  type: string,
  data: unknown,
): BoardState | null | undefined {
  if (!prev) return null;
  switch (type) {
    case "ticket_created": {
      const t = data as Ticket;
      if (!t || prev.tickets.some((x) => x.id === t.id)) return prev;
      return { ...prev, tickets: [...prev.tickets, t] };
    }
    case "ticket_updated":
    case "ticket_moved": {
      const t = data as Ticket;
      if (!t) return null;
      return { ...prev, tickets: prev.tickets.map((x) => (x.id === t.id ? t : x)) };
    }
    case "ticket_archived":
    case "ticket_deleted": {
      const t = data as Ticket;
      if (!t) return null;
      return {
        ...prev,
        tickets: prev.tickets.filter((x) => x.id !== t.id),
        sessions: prev.sessions.filter((s) => s.ticket_id !== t.id),
      };
    }
    case "ticket_unarchived": {
      const t = data as Ticket;
      if (!t || prev.tickets.some((x) => x.id === t.id)) return prev;
      return { ...prev, tickets: [...prev.tickets, t] };
    }
    case "session_updated": {
      const s = data as Session;
      if (!s) return null;
      const idx = prev.sessions.findIndex((x) => x.id === s.id);
      const sessions = idx === -1 ? [...prev.sessions, s] : prev.sessions.map((x) => (x.id === s.id ? s : x));
      return { ...prev, sessions };
    }
    case "ready":
      return null;
    default:
      return undefined;
  }
}
