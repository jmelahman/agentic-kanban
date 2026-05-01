import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useState } from "react";
import { api, subscribeBoard } from "./api/client";
import { ArchivedDrawer } from "./components/ArchivedDrawer";
import { Board } from "./components/Board";
import { BoardSettings } from "./components/BoardSettings";
import { CreateBoardForm } from "./components/CreateBoardForm";

export default function App() {
  const qc = useQueryClient();
  const boardsQ = useQuery({ queryKey: ["boards"], queryFn: api.listBoards });
  const [activeId, setActiveId] = useState<number | null>(null);
  const [streamStatus, setStreamStatus] = useState<"open" | "error" | "closed">("closed");
  const [showArchived, setShowArchived] = useState(false);
  const [showSettings, setShowSettings] = useState(false);

  const activeBoard = activeId != null ? boardsQ.data?.find((b) => b.id === activeId) ?? null : null;

  useEffect(() => {
    if (activeId == null && boardsQ.data && boardsQ.data.length > 0) {
      setActiveId(boardsQ.data[0].id);
    }
  }, [boardsQ.data, activeId]);

  useEffect(() => {
    if (activeId == null) return;
    return subscribeBoard(activeId, {
      onEvent: (type) => {
        qc.invalidateQueries({ queryKey: ["board", activeId] });
        if (type === "ticket_archived" || type === "ticket_deleted") {
          qc.invalidateQueries({ queryKey: ["archived", activeId] });
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
          {activeBoard && (
            <button
              className="rounded bg-zinc-800 p-1 text-zinc-300 hover:bg-zinc-700"
              onClick={() => setShowSettings(true)}
              aria-label="Board settings"
              title="Board settings"
            >
              <svg
                xmlns="http://www.w3.org/2000/svg"
                width="14"
                height="14"
                viewBox="0 0 24 24"
                fill="none"
                stroke="currentColor"
                strokeWidth="2"
                strokeLinecap="round"
                strokeLinejoin="round"
              >
                <circle cx="12" cy="12" r="3" />
                <path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 1 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-4 0v-.09a1.65 1.65 0 0 0-1-1.51 1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 1 1-2.83-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1 0-4h.09a1.65 1.65 0 0 0 1.51-1 1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 1 1 2.83-2.83l.06.06a1.65 1.65 0 0 0 1.82.33h.01a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 4 0v.09a1.65 1.65 0 0 0 1 1.51h.01a1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 1 1 2.83 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82v.01a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z" />
              </svg>
            </button>
          )}
          {activeId != null && (
            <button
              className="rounded bg-zinc-800 px-2 py-1 text-xs text-zinc-300 hover:bg-zinc-700"
              onClick={() => setShowArchived(true)}
            >
              archived
            </button>
          )}
          <CreateBoardForm
            onCreated={(b) => {
              qc.invalidateQueries({ queryKey: ["boards"] });
              setActiveId(b.id);
            }}
          />
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
      {activeId != null && showArchived && (
        <ArchivedDrawer boardId={activeId} onClose={() => setShowArchived(false)} />
      )}
      {activeBoard && showSettings && (
        <BoardSettings
          board={activeBoard}
          onClose={() => setShowSettings(false)}
          onDeleted={() => {
            setShowSettings(false);
            setActiveId(null);
          }}
        />
      )}
    </div>
  );
}
