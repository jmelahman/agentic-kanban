import { useMutation, useQueryClient } from "@tanstack/react-query";
import {
  ReactNode,
  useCallback,
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
} from "react";
import {
  api,
  formatApiError,
  MergeConfig,
  PR_STATE_COLOR,
  PRState,
  PullProgress,
  SyncConfig,
} from "@/api/client";
import { queryKeys } from "@/api/keys";
import {
  sessionStore,
  setTerminalSlot,
  usePullProgress,
  useSession,
  useTicket,
} from "@/store";
import { useToast } from "@/toast";
import { useClickOutside } from "@/hooks/useClickOutside";
import { useShortcut } from "@/keys/useShortcut";
import { Button, Spinner } from "./Button";
import { InfoPanel } from "./InfoPanel";
import { Tab } from "./Tab";
import { TasksPanel } from "./TasksPanel";

const TAB_ORDER = ["agent", "shell", "tasks", "info"] as const;
type TabId = (typeof TAB_ORDER)[number];

const TAB_KEY_PREFIX = "sessionPane.tab.";

function loadInitialTab(sessionId: number | null): TabId {
  if (sessionId == null || typeof localStorage === "undefined") return "agent";
  const raw = localStorage.getItem(TAB_KEY_PREFIX + sessionId);
  return (TAB_ORDER as readonly string[]).includes(raw ?? "")
    ? (raw as TabId)
    : "agent";
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

function enabledSyncStrategies(cfg: SyncConfig): SyncStrategy[] {
  const out: SyncStrategy[] = [];
  if (cfg.allow_rebase) out.push("rebase");
  if (cfg.allow_merge) out.push("merge");
  return out;
}

export type SessionViewProps = {
  ticketId: number;
  boardId: number;
  baseBranch: string;
  mergeConfig: MergeConfig;
  syncConfig: SyncConfig;
  sessionIdByTicket: Record<number, number>;
  onClose: () => void;
  // When false, this view's keyboard shortcuts are disabled. Multiple
  // SessionViews can be visible at once (overview canvas) — only the focused
  // one should react to start/stop/tab-cycling shortcuts.
  shortcutsEnabled?: boolean;
  // Trailing controls injected into the header (e.g. a fullscreen toggle in
  // the board pane). Rendered after the close button slot.
  headerExtras?: ReactNode;
};

// Inner content of the session pane: the action header, tab bar, and tab
// body. Owns its own ticket via props (no activeTicketStore dependency) and
// publishes its terminal slot DOM nodes into the global terminalSlotStore so
// the central TerminalsRoot can portal the right PtyTerminal in.
export function SessionView({
  ticketId,
  boardId,
  baseBranch,
  mergeConfig,
  syncConfig,
  sessionIdByTicket,
  onClose,
  shortcutsEnabled = true,
  headerExtras,
}: SessionViewProps) {
  const sessionId = sessionIdByTicket[ticketId] ?? null;
  const session = useSession(sessionId) ?? null;
  const ticket = useTicket(ticketId);
  const pullProgress = usePullProgress(sessionId);
  const qc = useQueryClient();
  const toast = useToast();
  const [tab, setTab] = useState<TabId>(() => loadInitialTab(sessionId));
  const tabsEnabled = shortcutsEnabled;
  useShortcut(
    "tab.next",
    () => setTab((t) => TAB_ORDER[(TAB_ORDER.indexOf(t) + 1) % TAB_ORDER.length]),
    { enabled: tabsEnabled },
  );
  useShortcut(
    "tab.prev",
    () =>
      setTab(
        (t) => TAB_ORDER[(TAB_ORDER.indexOf(t) - 1 + TAB_ORDER.length) % TAB_ORDER.length],
      ),
    { enabled: tabsEnabled },
  );
  const [syncMenuOpen, setSyncMenuOpen] = useState(false);
  const syncMenuRef = useRef<HTMLDivElement | null>(null);
  const [mergeMenuOpen, setMergeMenuOpen] = useState(false);
  const mergeMenuRef = useRef<HTMLDivElement | null>(null);

  const [compact, setCompact] = useState(false);
  const headerRef = useRef<HTMLDivElement | null>(null);
  const headerGhostRef = useRef<HTMLDivElement | null>(null);
  const autoStartedRef = useRef<number | null>(null);

  // Reload the persisted tab when the sessionId changes — each session has
  // its own tab memory under sessionPane.tab.{sessionId}.
  useEffect(() => {
    setTab(loadInitialTab(sessionId));
  }, [sessionId]);

  useEffect(() => {
    if (sessionId == null) return;
    localStorage.setItem(TAB_KEY_PREFIX + sessionId, tab);
  }, [tab, sessionId]);

  useClickOutside(syncMenuRef, () => setSyncMenuOpen(false), {
    enabled: syncMenuOpen,
  });
  useClickOutside(mergeMenuRef, () => setMergeMenuOpen(false), {
    enabled: mergeMenuOpen,
  });

  const boardKey = queryKeys.board(boardId);

  // Optimistic status updates flow straight into the session entity store —
  // only `Ticket`s for this session re-render. Returns the snapshot to roll
  // back on error.
  const optimisticStatus = (id: number, status: string) => {
    const prev = sessionStore.get(id);
    if (!prev) return { prev: null };
    sessionStore.set(id, { ...prev, status });
    return { prev };
  };
  const rollbackStatus = (id: number, prev: typeof session) => {
    if (prev) sessionStore.set(id, prev);
  };

  const startMut = useMutation({
    mutationFn: (id: number) => api.startSession(id),
    onMutate: (id) => optimisticStatus(id, "starting"),
    onError: (_err, id, ctx) => rollbackStatus(id, ctx?.prev ?? null),
  });
  const ensureMut = useMutation({
    mutationFn: () => api.ensureSession(ticketId),
    onSuccess: (created) => {
      sessionStore.set(created.id, created);
      qc.invalidateQueries({ queryKey: boardKey });
      startMut.mutate(created.id);
    },
    onError: () => {
      autoStartedRef.current = null;
    },
  });
  useEffect(() => {
    if (session) return;
    if (autoStartedRef.current === ticketId) return;
    autoStartedRef.current = ticketId;
    ensureMut.mutate();
  }, [ticketId, session, ensureMut]);
  const stopMut = useMutation({
    mutationFn: () => api.stopSession(session!.id),
    onMutate: () =>
      session ? optimisticStatus(session.id, "stopping") : { prev: null },
    onError: (_err, _vars, ctx) => {
      if (session) rollbackStatus(session.id, ctx?.prev ?? null);
    },
  });
  const restartMut = useMutation({
    mutationFn: () => api.restartSession(session!.id),
    onMutate: () =>
      session ? optimisticStatus(session.id, "starting") : { prev: null },
    onSuccess: () => toast.push("success", "container restarted"),
    onError: (_err, _vars, ctx) => {
      if (session) rollbackStatus(session.id, ctx?.prev ?? null);
    },
  });
  const archiveMut = useMutation({
    mutationFn: () => api.archiveTicket(ticketId),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: boardKey });
      onClose();
    },
  });
  const syncMut = useMutation({
    mutationFn: (strategy: SyncStrategy) => api.syncTicket(ticketId, strategy),
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
      api.mergeTicket(ticketId, strategy),
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
    mutationFn: () => api.doneTicket(ticketId),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: boardKey });
      onClose();
    },
  });

  // Per-ticket transient UI state. Switching tickets resets these so that
  // an in-flight action on the previous ticket doesn't leave the new
  // ticket's button stuck in its spinner state.
  useEffect(() => {
    setSyncMenuOpen(false);
    setMergeMenuOpen(false);
    startMut.reset();
    ensureMut.reset();
    stopMut.reset();
    restartMut.reset();
    archiveMut.reset();
    syncMut.reset();
    mergeMut.reset();
    doneMut.reset();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [ticketId]);

  useLayoutEffect(() => {
    const header = headerRef.current;
    const ghost = headerGhostRef.current;
    if (!header || !ghost) return;
    let raf: number | null = null;
    const measure = () => {
      raf = null;
      const h = headerRef.current;
      const g = headerGhostRef.current;
      if (!h || !g) return;
      setCompact(g.scrollWidth > h.clientWidth);
    };
    const schedule = () => {
      if (raf == null) raf = requestAnimationFrame(measure);
    };
    measure();
    const ro = new ResizeObserver(schedule);
    ro.observe(header);
    ro.observe(ghost);
    return () => {
      ro.disconnect();
      if (raf != null) cancelAnimationFrame(raf);
    };
  }, [ticketId]);

  // Register agent/shell slot DOM into the global terminalSlotStore so
  // TerminalsRoot can portal the matching PtyTerminal in. Re-runs when the
  // ticket id changes so a Board-view SessionView that swaps tickets
  // re-keys the slot. Cleans up on unmount or ticket change.
  const onAgentSlot = useCallback(
    (el: HTMLDivElement | null) => setTerminalSlot(ticketId, "agent", el),
    [ticketId],
  );
  const onShellSlot = useCallback(
    (el: HTMLDivElement | null) => setTerminalSlot(ticketId, "shell", el),
    [ticketId],
  );
  useEffect(() => {
    return () => {
      // Defensive cleanup if the ref callback didn't fire null (e.g. rapid
      // unmount during a portal swap).
      setTerminalSlot(ticketId, "agent", null);
      setTerminalSlot(ticketId, "shell", null);
    };
  }, [ticketId]);

  const mergeStrategies = enabledMergeStrategies(mergeConfig);
  const syncStrategies = enabledSyncStrategies(syncConfig);
  const status = session?.status;
  const isRunning =
    status && !["stopped", "error", "stopping"].includes(status);
  const canStart = session && !isRunning && status !== "starting";

  const renderHeaderContent = ({
    compact,
    interactive,
  }: {
    compact: boolean;
    interactive: boolean;
  }): ReactNode => (
    <>
      <span className="min-w-0 truncate font-medium">{ticket?.title}</span>
      <span className="whitespace-nowrap text-fg-muted">#{ticketId}</span>
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
          isRunning &&
          (compact ? (
            <Button
              variant="neutral"
              size="icon"
              onClick={() => restartMut.mutate()}
              disabled={restartMut.isPending}
              aria-label="restart container"
              title="restart container"
            >
              {restartMut.isPending ? <Spinner /> : <RestartIcon />}
            </Button>
          ) : (
            <Button
              variant="neutral"
              onClick={() => restartMut.mutate()}
              pending={restartMut.isPending}
              idleLabel="restart container"
              pendingLabel="restarting…"
              title="restart container"
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
              {syncMut.isPending ? <Spinner /> : <PullIcon />}
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
          <div className="relative" ref={interactive ? syncMenuRef : undefined}>
            {compact ? (
              <Button
                variant="neutral"
                size="icon"
                onClick={() => setSyncMenuOpen((v) => !v)}
                disabled={syncMut.isPending}
                aria-label="sync"
                title={`update from ${baseBranch}`}
              >
                {syncMut.isPending ? <Spinner /> : <PullIcon />}
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
            {interactive && syncMenuOpen && (
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
          <div
            className="relative"
            ref={interactive ? mergeMenuRef : undefined}
          >
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
            {interactive && mergeMenuOpen && (
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
        {headerExtras}
        <Button variant="neutral" size="icon" onClick={onClose}>
          ✕
        </Button>
      </div>
    </>
  );

  return (
    <div className="flex h-full min-h-0 flex-col">
      <div
        ref={headerRef}
        className="flex items-center gap-2 border-b border-border px-3 py-2 text-sm"
      >
        {renderHeaderContent({ compact, interactive: true })}
      </div>
      <div
        ref={headerGhostRef}
        aria-hidden
        className="pointer-events-none invisible absolute left-0 top-0 flex items-center gap-2 whitespace-nowrap px-3 py-2 text-sm"
      >
        {renderHeaderContent({ compact: false, interactive: false })}
      </div>
      {status === "starting" && (
        <PullProgressBanner progress={pullProgress ?? null} />
      )}
      {!session && ensureMut.isError && (
        <div className="border-b border-border bg-danger/10 px-3 py-2 text-xs text-danger">
          <div className="font-medium">Couldn't create session</div>
          <div className="mt-1 whitespace-pre-wrap break-words font-mono">
            {formatApiError(ensureMut.error)}
          </div>
        </div>
      )}
      <div className="flex border-b border-border text-sm">
        <Tab
          active={tab === "agent"}
          onClick={() => setTab("agent")}
          label="agent"
        />
        <Tab
          active={tab === "shell"}
          onClick={() => setTab("shell")}
          label="terminal"
        />
        <Tab
          active={tab === "tasks"}
          onClick={() => setTab("tasks")}
          label="tasks"
        />
        <Tab
          active={tab === "info"}
          onClick={() => setTab("info")}
          label="info"
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
        {tab === "info" &&
          (session ? (
            <InfoPanel session={session} />
          ) : (
            <p className="p-4 text-sm text-fg-muted">
              Create a session to see its info.
            </p>
          ))}
      </div>
    </div>
  );
}

// PullProgressBanner shows the live image pull state under the header while a
// session is starting. Falls back to an indeterminate "starting…" line when no
// progress events have arrived yet (image cached, or first event still in
// flight) so the bar isn't blank for the first ~200ms.
function PullProgressBanner({ progress }: { progress: PullProgress | null }) {
  const hasBytes = progress != null && progress.total > 0;
  const pct = hasBytes
    ? Math.min(
        100,
        Math.max(0, Math.round((progress.current / progress.total) * 100)),
      )
    : 0;
  const status = progress?.status?.toLowerCase() ?? "starting";
  return (
    <div className="border-b border-border bg-surface px-3 py-1.5 text-xs">
      <div className="flex items-center justify-between gap-2 text-fg-muted">
        <span className="truncate">
          {hasBytes ? `pulling image — ${status}` : "starting…"}
        </span>
        <span className="shrink-0 font-mono">
          {hasBytes
            ? `${formatBytes(progress.current)} / ${formatBytes(progress.total)} (${pct}%)`
            : ""}
        </span>
      </div>
      <div className="mt-1 h-1 w-full overflow-hidden rounded bg-border">
        {hasBytes ? (
          <div
            className="h-full bg-accent-500 transition-[width] duration-200 ease-out"
            style={{ width: `${pct}%` }}
          />
        ) : (
          <div className="h-full w-1/3 animate-pulse bg-accent-500/60" />
        )}
      </div>
    </div>
  );
}

function formatBytes(n: number): string {
  if (n < 1024) return `${n} B`;
  const units = ["KB", "MB", "GB", "TB"];
  let v = n / 1024;
  let i = 0;
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024;
    i++;
  }
  return `${v.toFixed(v >= 100 ? 0 : v >= 10 ? 1 : 2)} ${units[i]}`;
}

function PullIcon() {
  return (
    <svg {...iconProps()}>
      <circle cx="6" cy="4" r="2" />
      <line x1="6" y1="6" x2="6" y2="18" />
      <circle cx="6" cy="20" r="2" fill="currentColor" />
      <circle cx="18" cy="20" r="2" fill="currentColor" />
      <line x1="18" y1="18" x2="18" y2="8" />
      <path d="M 18 8 Q 18 4 13 4" fill="none" />
      <polyline points="13 1 10 4 13 7" />
    </svg>
  );
}

function RestartIcon() {
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
