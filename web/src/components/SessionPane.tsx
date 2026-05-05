import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useEffect, useRef, useState } from "react";
import {
  api,
  BoardState,
  MergeConfig,
  PRState,
  Session,
  SyncConfig,
} from "@/api/client";
import { queryKeys } from "@/api/keys";
import { useToast } from "@/toast";
import { FullscreenEnterIcon, FullscreenExitIcon } from "@/icons";
import { TerminalOrientation } from "@/hooks/useTerminalOrientation";
import { Button, Spinner } from "./Button";
import { TasksPanel } from "./TasksPanel";

const MIN_WIDTH = 320;
const MAX_WIDTH = 1600;
const DEFAULT_WIDTH = 640;
const WIDTH_STORAGE_KEY = "sessionPane.width";
const MIN_HEIGHT = 200;
const MAX_HEIGHT = 1200;
const DEFAULT_HEIGHT = 360;
const HEIGHT_STORAGE_KEY = "sessionPane.height";
// Below this pane width, sync/merge buttons collapse to icon-only.
const COMPACT_WIDTH = 680;

function loadInitialSize(
  key: string,
  fallback: number,
  min: number,
  max: number,
): number {
  const raw =
    typeof localStorage !== "undefined" ? localStorage.getItem(key) : null;
  const n = raw ? Number(raw) : NaN;
  if (!Number.isFinite(n)) return fallback;
  return Math.min(max, Math.max(min, n));
}

type MergeStrategy = "merge-commit" | "squash" | "rebase";

const MERGE_STRATEGY_LABELS: Record<MergeStrategy, string> = {
  "merge-commit": "create a merge commit",
  squash: "squash and merge",
  rebase: "rebase and merge",
};

function enabledMergeStrategies(cfg: MergeConfig): MergeStrategy[] {
  const out: MergeStrategy[] = [];
  if (cfg.allow_merge_commit) out.push("merge-commit");
  if (cfg.allow_squash) out.push("squash");
  if (cfg.allow_rebase) out.push("rebase");
  return out;
}

type SyncStrategy = "rebase" | "merge";

const SYNC_STRATEGY_LABELS: Record<SyncStrategy, string> = {
  rebase: "rebase from",
  merge: "merge from",
};

const PR_STATE_COLOR: Record<PRState, string> = {
  draft: "text-zinc-400",
  open: "text-emerald-400",
  merged: "text-purple-400",
  closed: "text-red-400",
};

function enabledSyncStrategies(cfg: SyncConfig): SyncStrategy[] {
  const out: SyncStrategy[] = [];
  if (cfg.allow_rebase) out.push("rebase");
  if (cfg.allow_merge) out.push("merge");
  return out;
}

export function SessionPane({
  boardId,
  baseBranch,
  mergeConfig,
  syncConfig,
  ticketId,
  session,
  onClose,
  onAgentSlot,
  onShellSlot,
  orientation,
}: {
  boardId: number;
  baseBranch: string;
  mergeConfig: MergeConfig;
  syncConfig: SyncConfig;
  ticketId: number | null;
  session: Session | null;
  onClose: () => void;
  onAgentSlot: (el: HTMLDivElement | null) => void;
  onShellSlot: (el: HTMLDivElement | null) => void;
  orientation: TerminalOrientation;
}) {
  const isHorizontal = orientation === "horizontal";
  const qc = useQueryClient();
  const toast = useToast();
  const [tab, setTab] = useState<"agent" | "shell" | "tasks">("agent");
  const [syncMenuOpen, setSyncMenuOpen] = useState(false);
  const syncMenuRef = useRef<HTMLDivElement | null>(null);
  const [mergeMenuOpen, setMergeMenuOpen] = useState(false);
  const mergeMenuRef = useRef<HTMLDivElement | null>(null);
  const [width, setWidth] = useState<number>(() =>
    loadInitialSize(WIDTH_STORAGE_KEY, DEFAULT_WIDTH, MIN_WIDTH, MAX_WIDTH),
  );
  const [height, setHeight] = useState<number>(() =>
    loadInitialSize(HEIGHT_STORAGE_KEY, DEFAULT_HEIGHT, MIN_HEIGHT, MAX_HEIGHT),
  );
  const [resizing, setResizing] = useState(false);
  const [fullscreen, setFullscreen] = useState(false);
  const autoStartedRef = useRef<number | null>(null);

  useEffect(() => {
    if (ticketId == null) return;
    const handler = (e: MouseEvent) => {
      const target = e.target as Element | null;
      if (!target?.closest("[data-board-area]")) return;
      // Ticket clicks switch the active ticket themselves; skip close to avoid a flicker.
      if (target.closest("[data-ticket-card]")) return;
      onClose();
    };
    window.addEventListener("mousedown", handler);
    return () => window.removeEventListener("mousedown", handler);
  }, [ticketId, onClose]);

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
    let pending: number | null = null;
    let nextSize = isHorizontal ? height : width;
    const apply = () => {
      pending = null;
      if (isHorizontal) setHeight(nextSize);
      else setWidth(nextSize);
    };
    const onMove = (e: MouseEvent) => {
      nextSize = isHorizontal
        ? Math.min(
            MAX_HEIGHT,
            Math.max(MIN_HEIGHT, window.innerHeight - e.clientY),
          )
        : Math.min(
            MAX_WIDTH,
            Math.max(MIN_WIDTH, window.innerWidth - e.clientX),
          );
      if (pending == null) pending = requestAnimationFrame(apply);
    };
    const onUp = () => setResizing(false);
    window.addEventListener("mousemove", onMove);
    window.addEventListener("mouseup", onUp);
    const prevCursor = document.body.style.cursor;
    const prevSelect = document.body.style.userSelect;
    document.body.style.cursor = isHorizontal ? "row-resize" : "col-resize";
    document.body.style.userSelect = "none";
    return () => {
      if (pending != null) cancelAnimationFrame(pending);
      window.removeEventListener("mousemove", onMove);
      window.removeEventListener("mouseup", onUp);
      document.body.style.cursor = prevCursor;
      document.body.style.userSelect = prevSelect;
    };
  }, [resizing, isHorizontal]);

  useEffect(() => {
    if (resizing) return;
    localStorage.setItem(WIDTH_STORAGE_KEY, String(width));
  }, [width, resizing]);

  useEffect(() => {
    if (resizing) return;
    localStorage.setItem(HEIGHT_STORAGE_KEY, String(height));
  }, [height, resizing]);

  useEffect(() => {
    if (!syncMenuOpen) return;
    const handler = (e: MouseEvent) => {
      if (!syncMenuRef.current?.contains(e.target as Node))
        setSyncMenuOpen(false);
    };
    window.addEventListener("mousedown", handler);
    return () => window.removeEventListener("mousedown", handler);
  }, [syncMenuOpen]);

  useEffect(() => {
    if (!mergeMenuOpen) return;
    const handler = (e: MouseEvent) => {
      if (!mergeMenuRef.current?.contains(e.target as Node))
        setMergeMenuOpen(false);
    };
    window.addEventListener("mousedown", handler);
    return () => window.removeEventListener("mousedown", handler);
  }, [mergeMenuOpen]);

  const boardKey = queryKeys.board(boardId);

  const optimisticStatus = (sessionId: number, status: string) => {
    const prev = qc.getQueryData<BoardState>(boardKey);
    if (!prev) return { prev };
    qc.setQueryData<BoardState>(boardKey, {
      ...prev,
      sessions: prev.sessions.map((s) =>
        s.id === sessionId ? { ...s, status } : s,
      ),
    });
    return { prev };
  };

  const startMut = useMutation({
    mutationFn: (id: number) => api.startSession(id),
    onMutate: (id) => optimisticStatus(id, "starting"),
    onSuccess: () => qc.invalidateQueries({ queryKey: boardKey }),
    onError: (_err, _vars, ctx) => {
      if (ctx?.prev) qc.setQueryData(boardKey, ctx.prev);
    },
  });
  const ensureMut = useMutation({
    mutationFn: () => api.ensureSession(ticketId!),
    onSuccess: (created) => {
      qc.invalidateQueries({ queryKey: boardKey });
      startMut.mutate(created.id);
    },
    onError: () => {
      autoStartedRef.current = null;
    },
  });
  useEffect(() => {
    if (ticketId == null || session) return;
    if (autoStartedRef.current === ticketId) return;
    autoStartedRef.current = ticketId;
    ensureMut.mutate();
  }, [ticketId, session, ensureMut]);
  const stopMut = useMutation({
    mutationFn: () => api.stopSession(session!.id),
    onMutate: () =>
      session ? optimisticStatus(session.id, "stopping") : { prev: undefined },
    onSuccess: () => qc.invalidateQueries({ queryKey: boardKey }),
    onError: (_err, _vars, ctx) => {
      if (ctx?.prev) qc.setQueryData(boardKey, ctx.prev);
    },
  });
  const archiveMut = useMutation({
    mutationFn: () => api.archiveTicket(ticketId!),
    onMutate: () => {
      const prev = qc.getQueryData<BoardState>(boardKey);
      if (prev) {
        qc.setQueryData<BoardState>(boardKey, {
          ...prev,
          tickets: prev.tickets.filter((t) => t.id !== ticketId),
        });
      }
      onClose();
      return { prev };
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: boardKey }),
    onError: (_err, _vars, ctx) => {
      if (ctx?.prev) qc.setQueryData(boardKey, ctx.prev);
    },
  });
  const syncMut = useMutation({
    mutationFn: (strategy: SyncStrategy) => api.syncTicket(ticketId!, strategy),
    onSuccess: (_data, strategy) => {
      setSyncMenuOpen(false);
      toast.push("success", `${strategy} from ${baseBranch} succeeded`);
      qc.invalidateQueries({ queryKey: boardKey });
    },
    onError: () => {
      setSyncMenuOpen(false);
    },
  });
  const mergeMut = useMutation({
    mutationFn: (strategy: MergeStrategy) =>
      api.mergeTicket(ticketId!, strategy),
    onSuccess: (_data, strategy) => {
      setMergeMenuOpen(false);
      toast.push("success", `${strategy} into ${baseBranch} succeeded`);
      qc.invalidateQueries({ queryKey: boardKey });
    },
    onError: () => {
      setMergeMenuOpen(false);
    },
  });
  const doneMut = useMutation({
    mutationFn: () => api.doneTicket(ticketId!),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: boardKey });
      onClose();
    },
  });

  if (ticketId == null) return null;
  const mergeStrategies = enabledMergeStrategies(mergeConfig);
  const syncStrategies = enabledSyncStrategies(syncConfig);
  const status = session?.status;
  const isRunning =
    status && !["stopped", "error", "stopping"].includes(status);
  const canStart = session && !isRunning && status !== "starting";
  const compact = !fullscreen && !isHorizontal && width < COMPACT_WIDTH;

  const paneClass = fullscreen
    ? "fixed inset-0 z-40 flex flex-col bg-bg"
    : isHorizontal
      ? "relative flex flex-col border-t border-border bg-bg"
      : "relative flex flex-col border-l border-border bg-bg";
  const paneStyle = fullscreen
    ? undefined
    : isHorizontal
      ? { height: `${height}px`, flex: `0 0 ${height}px` }
      : { width: `${width}px`, flex: `0 0 ${width}px` };

  return (
    <aside className={paneClass} style={paneStyle}>
      {!fullscreen && (
        <div
          role="separator"
          aria-orientation={isHorizontal ? "horizontal" : "vertical"}
          onMouseDown={(e) => {
            e.preventDefault();
            setResizing(true);
          }}
          onDoubleClick={() =>
            isHorizontal ? setHeight(DEFAULT_HEIGHT) : setWidth(DEFAULT_WIDTH)
          }
          className={
            isHorizontal
              ? `absolute left-0 top-0 z-20 h-1 w-full -translate-y-1/2 cursor-row-resize hover:bg-accent-500/40 ${
                  resizing ? "bg-accent-500/60" : ""
                }`
              : `absolute left-0 top-0 z-20 h-full w-1 -translate-x-1/2 cursor-col-resize hover:bg-accent-500/40 ${
                  resizing ? "bg-accent-500/60" : ""
                }`
          }
        />
      )}
      <div className="flex items-center gap-2 border-b border-border px-3 py-2 text-sm">
        <span className="font-medium">Ticket #{ticketId}</span>
        <span className="text-fg-muted">{session?.branch_name}</span>
        {session?.pr_number != null && session.pr_url && (
          <a
            href={session.pr_url}
            target="_blank"
            rel="noreferrer"
            className={`hover:underline ${
              session.pr_state
                ? (PR_STATE_COLOR[session.pr_state as PRState] ??
                  "text-fg-muted")
                : "text-fg-muted"
            }`}
            title={session.pr_state ? `PR ${session.pr_state}` : "pull request"}
          >
            #{session.pr_number}
          </a>
        )}
        <div className="ml-auto flex gap-2">
          {!session &&
            (compact ? (
              <Button
                variant="primary"
                size="icon"
                onClick={() => ensureMut.mutate()}
                disabled={ensureMut.isPending}
                aria-label="create session"
                title="create session"
              >
                {ensureMut.isPending ? <Spinner /> : <PlusIcon />}
              </Button>
            ) : (
              <Button
                variant="primary"
                onClick={() => ensureMut.mutate()}
                pending={ensureMut.isPending}
                idleLabel="create session"
                pendingLabel="creating session…"
              />
            ))}
          {canStart &&
            (compact ? (
              <Button
                variant="primary"
                size="icon"
                onClick={() => startMut.mutate(session.id)}
                disabled={startMut.isPending || status === "starting"}
                aria-label="start"
                title="start"
              >
                {startMut.isPending || status === "starting" ? (
                  <Spinner />
                ) : (
                  <PlayIcon />
                )}
              </Button>
            ) : (
              <Button
                variant="primary"
                onClick={() => startMut.mutate(session.id)}
                pending={startMut.isPending || status === "starting"}
                idleLabel="start"
                pendingLabel="starting…"
              />
            ))}
          {session &&
            isRunning &&
            (compact ? (
              <Button
                variant="secondary"
                size="icon"
                onClick={() => stopMut.mutate()}
                disabled={stopMut.isPending || status === "stopping"}
                aria-label="stop"
                title="stop"
              >
                {stopMut.isPending || status === "stopping" ? (
                  <Spinner />
                ) : (
                  <StopIcon />
                )}
              </Button>
            ) : (
              <Button
                variant="secondary"
                onClick={() => stopMut.mutate()}
                pending={stopMut.isPending || status === "stopping"}
                idleLabel="stop"
                pendingLabel="stopping…"
              />
            ))}
          {session &&
            syncStrategies.length === 1 &&
            (compact ? (
              <Button
                variant="neutral"
                size="icon"
                onClick={() => syncMut.mutate(syncStrategies[0])}
                disabled={syncMut.isPending}
                aria-label={`${SYNC_STRATEGY_LABELS[syncStrategies[0]]} ${baseBranch}`}
                title={`${SYNC_STRATEGY_LABELS[syncStrategies[0]]} ${baseBranch}`}
              >
                {syncMut.isPending ? <Spinner /> : <SyncIcon />}
              </Button>
            ) : (
              <Button
                variant="neutral"
                onClick={() => syncMut.mutate(syncStrategies[0])}
                pending={syncMut.isPending}
                idleLabel={`${SYNC_STRATEGY_LABELS[syncStrategies[0]]} ${baseBranch}`}
                pendingLabel="syncing…"
                title={`update from ${baseBranch}`}
              />
            ))}
          {session && syncStrategies.length > 1 && (
            <div className="relative" ref={syncMenuRef}>
              {compact ? (
                <Button
                  variant="neutral"
                  size="icon"
                  onClick={() => setSyncMenuOpen((v) => !v)}
                  disabled={syncMut.isPending}
                  aria-label="sync"
                  title={`update from ${baseBranch}`}
                >
                  {syncMut.isPending ? <Spinner /> : <SyncIcon />}
                </Button>
              ) : (
                <Button
                  variant="neutral"
                  onClick={() => setSyncMenuOpen((v) => !v)}
                  pending={syncMut.isPending}
                  idleLabel="sync ▾"
                  pendingLabel="syncing…"
                  title={`update from ${baseBranch}`}
                />
              )}
              {syncMenuOpen && (
                <div className="absolute right-0 top-full z-10 mt-1 w-56 rounded border border-border bg-surface p-1 text-xs shadow-lg">
                  {syncStrategies.map((s) => (
                    <Button
                      key={s}
                      variant="ghost"
                      className="block w-full text-left"
                      onClick={() => syncMut.mutate(s)}
                    >
                      {SYNC_STRATEGY_LABELS[s]}{" "}
                      <span className="font-mono">{baseBranch}</span>
                    </Button>
                  ))}
                </div>
              )}
            </div>
          )}
          {session &&
            mergeStrategies.length === 1 &&
            (compact ? (
              <Button
                variant="neutral"
                size="icon"
                onClick={() => mergeMut.mutate(mergeStrategies[0])}
                disabled={mergeMut.isPending}
                aria-label={MERGE_STRATEGY_LABELS[mergeStrategies[0]]}
                title={MERGE_STRATEGY_LABELS[mergeStrategies[0]]}
              >
                {mergeMut.isPending ? <Spinner /> : <MergeIcon />}
              </Button>
            ) : (
              <Button
                variant="neutral"
                onClick={() => mergeMut.mutate(mergeStrategies[0])}
                pending={mergeMut.isPending}
                idleLabel={MERGE_STRATEGY_LABELS[mergeStrategies[0]]}
                pendingLabel="merging…"
                title={`integrate into ${baseBranch}`}
              />
            ))}
          {session && mergeStrategies.length > 1 && (
            <div className="relative" ref={mergeMenuRef}>
              {compact ? (
                <Button
                  variant="neutral"
                  size="icon"
                  onClick={() => setMergeMenuOpen((v) => !v)}
                  disabled={mergeMut.isPending}
                  aria-label="merge"
                  title={`integrate into ${baseBranch}`}
                >
                  {mergeMut.isPending ? <Spinner /> : <MergeIcon />}
                </Button>
              ) : (
                <Button
                  variant="neutral"
                  onClick={() => setMergeMenuOpen((v) => !v)}
                  pending={mergeMut.isPending}
                  idleLabel="merge ▾"
                  pendingLabel="merging…"
                  title={`integrate into ${baseBranch}`}
                />
              )}
              {mergeMenuOpen && (
                <div className="absolute right-0 top-full z-10 mt-1 w-64 rounded border border-border bg-surface p-1 text-xs shadow-lg">
                  {mergeStrategies.map((s) => (
                    <Button
                      key={s}
                      variant="ghost"
                      className="block w-full text-left"
                      onClick={() => mergeMut.mutate(s)}
                    >
                      {MERGE_STRATEGY_LABELS[s]}
                    </Button>
                  ))}
                </div>
              )}
            </div>
          )}
          {compact ? (
            <Button
              variant="neutral"
              size="icon"
              onClick={() => archiveMut.mutate()}
              disabled={archiveMut.isPending}
              aria-label="archive"
              title="archive"
            >
              {archiveMut.isPending ? <Spinner /> : <ArchiveIcon />}
            </Button>
          ) : (
            <Button
              variant="neutral"
              onClick={() => archiveMut.mutate()}
              pending={archiveMut.isPending}
              idleLabel="archive"
              pendingLabel="archiving…"
            />
          )}
          {compact ? (
            <Button
              variant="primary"
              size="icon"
              onClick={() => doneMut.mutate()}
              disabled={doneMut.isPending}
              aria-label="done"
              title="mark as done"
            >
              {doneMut.isPending ? <Spinner /> : <CheckIcon />}
            </Button>
          ) : (
            <Button
              variant="primary"
              onClick={() => doneMut.mutate()}
              pending={doneMut.isPending}
              idleLabel="done"
              pendingLabel="finishing…"
              title="mark as done"
            />
          )}
          <Button
            variant="neutral"
            size="icon"
            onClick={() => setFullscreen((v) => !v)}
            aria-label={fullscreen ? "Exit fullscreen" : "Fullscreen"}
            title={fullscreen ? "Exit fullscreen (Esc)" : "Fullscreen"}
          >
            {fullscreen ? <FullscreenExitIcon /> : <FullscreenEnterIcon />}
          </Button>
          <Button variant="neutral" size="icon" onClick={onClose}>
            ✕
          </Button>
        </div>
      </div>
      <div className="flex border-b border-border text-sm">
        <Tab
          active={tab === "agent"}
          onClick={() => setTab("agent")}
          label="agent"
        />
        <Tab
          active={tab === "shell"}
          onClick={() => setTab("shell")}
          label="shell"
        />
        <Tab
          active={tab === "tasks"}
          onClick={() => setTab("tasks")}
          label="tasks"
        />
      </div>
      <div className="min-h-0 flex-1 bg-bg">
        {tab === "agent" && (
          <div className="h-full">
            {session && isRunning ? (
              <div ref={onAgentSlot} className="h-full w-full" />
            ) : (
              <p className="p-4 text-sm text-fg-muted">
                Start the session to attach the agent.
              </p>
            )}
          </div>
        )}
        {tab === "shell" && (
          <div className="h-full">
            {session && isRunning ? (
              <div ref={onShellSlot} className="h-full w-full" />
            ) : (
              <p className="p-4 text-sm text-fg-muted">
                Start the session to attach a shell.
              </p>
            )}
          </div>
        )}
        {tab === "tasks" && session && (
          <TasksPanel session={session} boardId={boardId} />
        )}
      </div>
    </aside>
  );
}

function SyncIcon() {
  return (
    <svg {...iconProps()} viewBox="-3 -3 30 30">
      <path d="M3 12a9 9 0 0 1 15-6.7L21 8" />
      <path d="M21 3v5h-5" />
      <path d="M21 12a9 9 0 0 1-15 6.7L3 16" />
      <path d="M3 21v-5h5" />
    </svg>
  );
}

function MergeIcon() {
  return (
    <svg {...iconProps()} viewBox="-3 -3 30 30">
      <circle cx="6" cy="6" r="3" />
      <circle cx="6" cy="18" r="3" />
      <circle cx="18" cy="9" r="3" />
      <path d="M6 9v6" />
      <path d="M6 15a9 9 0 0 0 9-6" />
    </svg>
  );
}

function iconProps() {
  return {
    xmlns: "http://www.w3.org/2000/svg",
    width: "14",
    height: "14",
    viewBox: "0 0 24 24",
    fill: "none",
    stroke: "currentColor",
    strokeWidth: "2",
    strokeLinecap: "round" as const,
    strokeLinejoin: "round" as const,
  };
}

function PlayIcon() {
  return (
    <svg {...iconProps()} fill="currentColor" stroke="none">
      <path d="M6 4l14 8-14 8V4z" />
    </svg>
  );
}

function StopIcon() {
  return (
    <svg {...iconProps()} fill="currentColor" stroke="none">
      <rect x="6" y="6" width="12" height="12" rx="1" />
    </svg>
  );
}

function PlusIcon() {
  return (
    <svg {...iconProps()}>
      <line x1="12" y1="5" x2="12" y2="19" />
      <line x1="5" y1="12" x2="19" y2="12" />
    </svg>
  );
}

function ArchiveIcon() {
  return (
    <svg {...iconProps()}>
      <rect x="3" y="3" width="18" height="5" rx="1" />
      <path d="M5 8v11a1 1 0 0 0 1 1h12a1 1 0 0 0 1-1V8" />
      <line x1="10" y1="13" x2="14" y2="13" />
    </svg>
  );
}

function CheckIcon() {
  return (
    <svg {...iconProps()}>
      <polyline points="20 6 9 17 4 12" />
    </svg>
  );
}

function Tab({
  active,
  onClick,
  label,
}: {
  active: boolean;
  onClick: () => void;
  label: string;
}) {
  return (
    <button
      onClick={onClick}
      className={`px-3 py-2 transition-colors duration-150 ${active ? "border-b-2 border-accent-500 text-fg" : "text-fg-muted hover:text-fg"}`}
    >
      {label}
    </button>
  );
}
