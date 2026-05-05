import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useState } from "react";
import { api, subscribeBoard } from "@/api/client";
import { queryKeys } from "@/api/keys";
import { AppSettings } from "@/components/AppSettings";
import { ArchivedDrawer } from "@/components/ArchivedDrawer";
import { Board } from "@/components/Board";
import { BoardSettings } from "@/components/BoardSettings";
import { Button } from "@/components/Button";
import { CreateBoardForm } from "@/components/CreateBoardForm";
import { CogIcon, MenuIcon } from "@/icons";
import { readActiveBoardId, writeActiveBoardId } from "@/storage";

export default function App() {
  const qc = useQueryClient();
  const boardsQ = useQuery({ queryKey: queryKeys.boards, queryFn: api.listBoards });
  const [activeId, setActiveId] = useState<number | null>(null);
  const [streamStatus, setStreamStatus] = useState<"open" | "error" | "closed">("closed");
  const [showArchived, setShowArchived] = useState(false);
  const [showSettings, setShowSettings] = useState(false);
  const [showAppSettings, setShowAppSettings] = useState(false);

  const activeBoard = activeId != null ? boardsQ.data?.find((b) => b.id === activeId) ?? null : null;

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

  useEffect(() => {
    if (activeId == null) return;
    return subscribeBoard(activeId, {
      onEvent: (type) => {
        qc.invalidateQueries({ queryKey: queryKeys.board(activeId) });
        if (type === "ticket_archived" || type === "ticket_unarchived" || type === "ticket_deleted") {
          qc.invalidateQueries({ queryKey: queryKeys.archived(activeId) });
        }
      },
      onStatus: setStreamStatus,
    });
  }, [activeId, qc]);

  return (
    <div className="flex h-full flex-col">
      <header className="flex items-center gap-4 border-b border-zinc-800 px-4 py-2">
        <h1 className="text-lg font-semibold">Kanban</h1>
        <select
          className="rounded bg-zinc-900 px-2 py-1 text-sm"
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
              size="sm"
              className="inline-flex h-7 w-7 items-center justify-center"
              onClick={() => setShowSettings(true)}
              aria-label="Board settings"
              title="Board settings"
            >
              <CogIcon />
            </Button>
          )}
          <Button
            variant="neutral"
            size="sm"
            className="inline-flex h-7 w-7 items-center justify-center"
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
        {activeId != null ? <Board boardId={activeId} /> : <p className="p-4 text-sm text-zinc-400">No board selected.</p>}
      </main>
      {activeId != null && (
        <ArchivedDrawer open={showArchived} boardId={activeId} onClose={() => setShowArchived(false)} />
      )}
      {activeBoard && (
        <BoardSettings
          open={showSettings}
          board={activeBoard}
          onClose={() => setShowSettings(false)}
          onDeleted={() => {
            setShowSettings(false);
            setActiveId(null);
          }}
        />
      )}
      <AppSettings open={showAppSettings} onClose={() => setShowAppSettings(false)} />
    </div>
  );
}
