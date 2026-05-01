import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useEffect, useRef, useState } from "react";
import { api, ApiError, BoardState, Session } from "../api/client";
import { useToast } from "../toast";
import { PendingButton } from "./PendingButton";
import { TasksPanel } from "./TasksPanel";

const MIN_WIDTH = 320;
const MAX_WIDTH = 1600;
const DEFAULT_WIDTH = 640;
const WIDTH_STORAGE_KEY = "sessionPane.width";

function loadInitialWidth(): number {
  const raw = typeof localStorage !== "undefined" ? localStorage.getItem(WIDTH_STORAGE_KEY) : null;
  const n = raw ? Number(raw) : NaN;
  if (!Number.isFinite(n)) return DEFAULT_WIDTH;
  return Math.min(MAX_WIDTH, Math.max(MIN_WIDTH, n));
}

function errorMessage(err: unknown): string {
  if (err instanceof ApiError) return err.message;
  if (err instanceof Error) return err.message;
  return String(err);
}

export function SessionPane({
  boardId,
  baseBranch,
  ticketId,
  session,
  onClose,
  onTerminalSlot,
}: {
  boardId: number;
  baseBranch: string;
  ticketId: number | null;
  session: Session | null;
  onClose: () => void;
  onTerminalSlot: (el: HTMLDivElement | null) => void;
}) {
  const qc = useQueryClient();
  const toast = useToast();
  const [tab, setTab] = useState<"terminal" | "tasks">("terminal");
  const [syncMenuOpen, setSyncMenuOpen] = useState(false);
  const syncMenuRef = useRef<HTMLDivElement | null>(null);
  const [width, setWidth] = useState<number>(() => loadInitialWidth());
  const [resizing, setResizing] = useState(false);
  const [fullscreen, setFullscreen] = useState(false);

  useEffect(() => {
    if (!fullscreen) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") setFullscreen(false);
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [fullscreen]);

  useEffect(() => {
    if (!resizing) return;
    const onMove = (e: MouseEvent) => {
      const next = window.innerWidth - e.clientX;
      setWidth(Math.min(MAX_WIDTH, Math.max(MIN_WIDTH, next)));
    };
    const onUp = () => setResizing(false);
    window.addEventListener("mousemove", onMove);
    window.addEventListener("mouseup", onUp);
    const prevCursor = document.body.style.cursor;
    const prevSelect = document.body.style.userSelect;
    document.body.style.cursor = "col-resize";
    document.body.style.userSelect = "none";
    return () => {
      window.removeEventListener("mousemove", onMove);
      window.removeEventListener("mouseup", onUp);
      document.body.style.cursor = prevCursor;
      document.body.style.userSelect = prevSelect;
    };
  }, [resizing]);

  useEffect(() => {
    localStorage.setItem(WIDTH_STORAGE_KEY, String(width));
  }, [width]);

  useEffect(() => {
    if (!syncMenuOpen) return;
    const handler = (e: MouseEvent) => {
      if (!syncMenuRef.current?.contains(e.target as Node)) setSyncMenuOpen(false);
    };
    window.addEventListener("mousedown", handler);
    return () => window.removeEventListener("mousedown", handler);
  }, [syncMenuOpen]);

  const boardKey = ["board", boardId] as const;

  const optimisticStatus = (sessionId: number, status: string) => {
    const prev = qc.getQueryData<BoardState>(boardKey);
    if (!prev) return { prev };
    qc.setQueryData<BoardState>(boardKey, {
      ...prev,
      sessions: prev.sessions.map((s) => (s.id === sessionId ? { ...s, status } : s)),
    });
    return { prev };
  };

  const ensureMut = useMutation({
    mutationFn: () => api.ensureSession(ticketId!),
    onSuccess: () => qc.invalidateQueries({ queryKey: boardKey }),
    onError: (err) => toast.push("error", errorMessage(err)),
  });
  const startMut = useMutation({
    mutationFn: () => api.startSession(session!.id),
    onMutate: () => (session ? optimisticStatus(session.id, "starting") : { prev: undefined }),
    onSuccess: () => qc.invalidateQueries({ queryKey: boardKey }),
    onError: (err, _vars, ctx) => {
      if (ctx?.prev) qc.setQueryData(boardKey, ctx.prev);
      toast.push("error", errorMessage(err));
    },
  });
  const stopMut = useMutation({
    mutationFn: () => api.stopSession(session!.id),
    onMutate: () => (session ? optimisticStatus(session.id, "stopping") : { prev: undefined }),
    onSuccess: () => qc.invalidateQueries({ queryKey: boardKey }),
    onError: (err, _vars, ctx) => {
      if (ctx?.prev) qc.setQueryData(boardKey, ctx.prev);
      toast.push("error", errorMessage(err));
    },
  });
  const archiveMut = useMutation({
    mutationFn: () => api.archiveTicket(ticketId!),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: boardKey });
      onClose();
    },
    onError: (err) => toast.push("error", errorMessage(err)),
  });
  const syncMut = useMutation({
    mutationFn: (strategy: "rebase" | "merge") => api.syncTicket(ticketId!, strategy),
    onSuccess: (_data, strategy) => {
      setSyncMenuOpen(false);
      toast.push("success", `${strategy} from ${baseBranch} succeeded`);
      qc.invalidateQueries({ queryKey: boardKey });
    },
    onError: (err) => {
      setSyncMenuOpen(false);
      toast.push("error", errorMessage(err));
    },
  });

  if (ticketId == null) return null;
  const status = session?.status;
  const isRunning = status && !["stopped", "error", "stopping"].includes(status);
  const canStart = session && !isRunning && status !== "starting";

  return (
    <aside
      className={
        fullscreen
          ? "fixed inset-0 z-40 flex flex-col bg-zinc-950"
          : "relative flex flex-col border-l border-zinc-800 bg-zinc-950"
      }
      style={fullscreen ? undefined : { width: `${width}px`, flex: `0 0 ${width}px` }}
    >
      {!fullscreen && (
        <div
          role="separator"
          aria-orientation="vertical"
          onMouseDown={(e) => {
            e.preventDefault();
            setResizing(true);
          }}
          onDoubleClick={() => setWidth(DEFAULT_WIDTH)}
          className={`absolute left-0 top-0 z-20 h-full w-1 -translate-x-1/2 cursor-col-resize hover:bg-red-500/40 ${
            resizing ? "bg-red-500/60" : ""
          }`}
        />
      )}
      <div className="flex items-center gap-2 border-b border-zinc-800 px-3 py-2 text-sm">
        <span className="font-medium">Ticket #{ticketId}</span>
        <span className="text-zinc-400">{session?.branch_name}</span>
        <div className="ml-auto flex gap-2">
          {!session && (
            <PendingButton
              className="rounded bg-red-700 px-2 py-1 disabled:opacity-60"
              onClick={() => ensureMut.mutate()}
              pending={ensureMut.isPending}
              idleLabel="create session"
              pendingLabel="creating session…"
            />
          )}
          {canStart && (
            <PendingButton
              className="rounded bg-red-700 px-2 py-1 disabled:opacity-60"
              onClick={() => startMut.mutate()}
              pending={startMut.isPending || status === "starting"}
              idleLabel="start"
              pendingLabel="starting…"
            />
          )}
          {session && isRunning && (
            <PendingButton
              className="rounded bg-zinc-700 px-2 py-1 disabled:opacity-60"
              onClick={() => stopMut.mutate()}
              pending={stopMut.isPending || status === "stopping"}
              idleLabel="stop"
              pendingLabel="stopping…"
            />
          )}
          {session && (
            <div className="relative" ref={syncMenuRef}>
              <PendingButton
                className="rounded bg-zinc-800 px-2 py-1 text-zinc-300 disabled:opacity-50"
                onClick={() => setSyncMenuOpen((v) => !v)}
                pending={syncMut.isPending}
                idleLabel="sync ▾"
                pendingLabel="syncing…"
                title={`update from ${baseBranch}`}
              />
              {syncMenuOpen && (
                <div className="absolute right-0 top-full z-10 mt-1 w-56 rounded border border-zinc-700 bg-zinc-900 p-1 text-xs shadow-lg">
                  <button
                    className="block w-full rounded px-2 py-1 text-left hover:bg-zinc-800"
                    onClick={() => syncMut.mutate("rebase")}
                  >
                    rebase from <span className="font-mono">{baseBranch}</span>
                  </button>
                  <button
                    className="block w-full rounded px-2 py-1 text-left hover:bg-zinc-800"
                    onClick={() => syncMut.mutate("merge")}
                  >
                    merge from <span className="font-mono">{baseBranch}</span>
                  </button>
                </div>
              )}
            </div>
          )}
          <PendingButton
            className="rounded bg-zinc-800 px-2 py-1 text-zinc-300 disabled:opacity-60"
            onClick={() => archiveMut.mutate()}
            pending={archiveMut.isPending}
            idleLabel="archive"
            pendingLabel="archiving…"
          />
          <button
            className="rounded p-1 text-zinc-400 hover:bg-zinc-800 hover:text-zinc-200"
            onClick={() => setFullscreen((v) => !v)}
            aria-label={fullscreen ? "Exit fullscreen" : "Fullscreen"}
            title={fullscreen ? "Exit fullscreen (Esc)" : "Fullscreen"}
          >
            {fullscreen ? (
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
                <path d="M8 3v3a2 2 0 0 1-2 2H3" />
                <path d="M21 8h-3a2 2 0 0 1-2-2V3" />
                <path d="M3 16h3a2 2 0 0 1 2 2v3" />
                <path d="M16 21v-3a2 2 0 0 1 2-2h3" />
              </svg>
            ) : (
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
                <path d="M3 8V5a2 2 0 0 1 2-2h3" />
                <path d="M21 8V5a2 2 0 0 0-2-2h-3" />
                <path d="M3 16v3a2 2 0 0 0 2 2h3" />
                <path d="M21 16v3a2 2 0 0 1-2 2h-3" />
              </svg>
            )}
          </button>
          <button className="text-zinc-400" onClick={onClose}>
            ✕
          </button>
        </div>
      </div>
      <div className="flex border-b border-zinc-800 text-sm">
        <Tab active={tab === "terminal"} onClick={() => setTab("terminal")} label="terminal" />
        <Tab active={tab === "tasks"} onClick={() => setTab("tasks")} label="tasks" />
      </div>
      <div className="min-h-0 flex-1">
        {tab === "terminal" && (
          <div className="h-full">
            {session && isRunning ? (
              <div ref={onTerminalSlot} className="h-full w-full" />
            ) : (
              <p className="p-4 text-sm text-zinc-400">Start the session to attach a terminal.</p>
            )}
          </div>
        )}
        {tab === "tasks" && session && <TasksPanel session={session} boardId={boardId} />}
      </div>
    </aside>
  );
}

function Tab({ active, onClick, label }: { active: boolean; onClick: () => void; label: string }) {
  return (
    <button
      onClick={onClick}
      className={`px-3 py-2 ${active ? "border-b-2 border-red-500 text-zinc-100" : "text-zinc-400 hover:text-zinc-200"}`}
    >
      {label}
    </button>
  );
}
