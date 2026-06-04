import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useState } from "react";
import { api } from "@/api/client";
import { queryKeys } from "@/api/keys";
import { activeTicketStore } from "@/store";
import { AppSettings } from "@/components/AppSettings";
import { ArchivedDrawer } from "@/components/ArchivedDrawer";
import { Board } from "@/components/Board";
import { BoardSettings } from "@/components/BoardSettings";
import { Button } from "@/components/Button";
import { CreateBoardModal } from "@/components/CreateBoardForm";
import { DevToolbar } from "@/components/devToolbar/DevToolbar";
import { setDevToolbarPrefs, useDevToolbarPrefs } from "@/components/devToolbar/preferences";
import { HeaderMobileMenu } from "@/components/HeaderMobileMenu";
import { Overview } from "@/components/Overview/Overview";
import { SessionCounter } from "@/components/SessionCounter";
import { Tab } from "@/components/Tab";
import { TerminalsRoot } from "@/components/TerminalsRoot";
import { useAccent } from "@/hooks/useAccent";
import { useBoardSubscription, type StreamStatus } from "@/hooks/useBoardSubscription";
import { useContrast } from "@/hooks/useContrast";
import { useDevToolbarEnabled } from "@/hooks/useDevToolbarEnabled";
import { useMediaQuery } from "@/hooks/useMediaQuery";
import { useThemeMode } from "@/hooks/useThemeMode";
import { ArchiveIcon, CogIcon, GaugeIcon, HelpIcon, MenuIcon, PlusIcon } from "@/icons";
import { useShortcut } from "@/keys/useShortcut";
import { readActiveBoardId, writeActiveBoardId } from "@/storage";

const VIEW_KEY = "app.view";
type AppView = "board" | "overview";

function loadInitialView(): AppView {
  try {
    const raw = localStorage.getItem(VIEW_KEY);
    return raw === "overview" ? "overview" : "board";
  } catch {
    return "board";
  }
}

// Pride Month: paint the wordmark in animated rainbow during June (month index 5).
function isPrideMonth(): boolean {
  return new Date().getMonth() === 5;
}

export default function App() {
  useThemeMode();
  useContrast();
  useAccent();
  const qc = useQueryClient();
  const boardsQ = useQuery({
    queryKey: queryKeys.boards,
    queryFn: api.listBoards,
  });
  const [activeId, setActiveId] = useState<number | null>(null);
  const [view, setView] = useState<AppView>(loadInitialView);
  const [streamStatus, setStreamStatus] = useState<StreamStatus>("closed");
  // EventSource fires a transient `onerror` during the initial connection
  // before `onopen` resolves, which would flash the banner on every refresh.
  // Only surface the banner if the error persists past a short grace window.
  const [showStreamError, setShowStreamError] = useState(false);
  const [showArchived, setShowArchived] = useState(false);
  const [showSettings, setShowSettings] = useState(false);
  const [showAppSettings, setShowAppSettings] = useState(false);
  const [createBoardOpen, setCreateBoardOpen] = useState(false);
  const isMobile = useMediaQuery("(max-width: 639px)");
  // Opt-in developer toolbar: the config flag (from `.kanban.toml`) gates
  // whether the header button and widget exist at all; the persisted `open`
  // pref controls whether the widget is currently shown.
  const devToolbarEnabled = useDevToolbarEnabled();
  const devToolbarPrefs = useDevToolbarPrefs();
  const toggleDevToolbar = () =>
    setDevToolbarPrefs({ ...devToolbarPrefs, open: !devToolbarPrefs.open });

  const onBoardCreated = (b: { id: number }) => {
    qc.invalidateQueries({ queryKey: queryKeys.boards });
    setActiveId(b.id);
    setView("board");
  };

  const activeBoard =
    activeId != null ? (boardsQ.data?.find((b) => b.id === activeId) ?? null) : null;
  const noBoards = boardsQ.data?.length === 0;

  // When the board's mount path is a visible git repo but no repo_path is
  // linked, badge the settings icon to nudge the user toward enabling git
  // features. Skipped when the path isn't visible to the kanban container
  // (state="unknown") — we can't know either way.
  const mountPath = activeBoard?.mount_path ?? "";
  const repoPath = activeBoard?.repo_path ?? "";
  const fsCheckQ = useQuery({
    queryKey: ["fsCheck", mountPath],
    queryFn: () => api.fsCheck(mountPath),
    enabled: !!mountPath && !repoPath,
    staleTime: 30_000,
  });
  const suggestRepoLink = !repoPath && fsCheckQ.data?.state === "git";

  useEffect(() => {
    if (activeId == null && boardsQ.data && boardsQ.data.length > 0) {
      const remembered = readActiveBoardId();
      const fallback = boardsQ.data[0].id;
      setActiveId(
        remembered != null && boardsQ.data.some((b) => b.id === remembered) ? remembered : fallback,
      );
    }
  }, [boardsQ.data, activeId]);

  useEffect(() => {
    if (activeId != null) writeActiveBoardId(activeId);
    // Selection lives outside the board; clear it when the board changes so a
    // ticket-id from board A doesn't leak into board B's pane.
    activeTicketStore.set(null);
  }, [activeId]);

  useEffect(() => {
    if (streamStatus !== "error") {
      setShowStreamError(false);
      return;
    }
    const t = setTimeout(() => setShowStreamError(true), 1500);
    return () => clearTimeout(t);
  }, [streamStatus]);

  useEffect(() => {
    try {
      localStorage.setItem(VIEW_KEY, view);
    } catch {
      // ignore
    }
  }, [view]);

  const cycleBoard = (delta: 1 | -1) => {
    const boards = boardsQ.data;
    if (!boards || boards.length < 2) return;
    const idx = boards.findIndex((b) => b.id === activeId);
    if (idx < 0) return;
    const next = (idx + delta + boards.length) % boards.length;
    setActiveId(boards[next].id);
  };
  useShortcut("board.next", () => cycleBoard(1), { enabled: view === "board" });
  useShortcut("board.prev", () => cycleBoard(-1), {
    enabled: view === "board",
  });
  useShortcut("board.create", () => setCreateBoardOpen(true));

  // Drives the connection-status banner. Only meaningful in the board view —
  // overview manages its own per-board subscriptions inside the tree, and a
  // single banner across N streams would be misleading.
  useBoardSubscription(view === "board" ? activeId : null, setStreamStatus);

  return (
    <div className="flex h-full min-w-0 flex-col">
      <header className="flex flex-wrap items-center gap-x-3 gap-y-2 border-b border-border px-3 py-2">
        <h1
          className={`hidden text-lg font-semibold sm:block ${isPrideMonth() ? "rainbow-text" : ""}`}
        >
          Kanban
        </h1>
        <nav className="flex self-stretch -my-2">
          <Tab active={view === "overview"} onClick={() => setView("overview")} label="overview" />
          <Tab active={view === "board"} onClick={() => setView("board")} label="board" />
        </nav>
        {view === "board" && (
          <select
            className="min-w-0 max-w-[40vw] cursor-pointer rounded bg-surface px-2 py-1 text-sm"
            value={activeId ?? ""}
            onChange={(e) => setActiveId(e.target.value ? Number(e.target.value) : null)}
          >
            <option value="">— select board —</option>
            {(boardsQ.data ?? []).map((b) => (
              <option key={b.id} value={b.id}>
                {b.name}
              </option>
            ))}
          </select>
        )}
        {isMobile ? (
          <div className="ml-auto flex items-center gap-2">
            <SessionCounter />
            <HeaderMobileMenu
              onNewBoard={() => setCreateBoardOpen(true)}
              onArchived={view === "board" && activeId != null ? () => setShowArchived(true) : null}
              onBoardSettings={view === "board" && activeBoard ? () => setShowSettings(true) : null}
              onAppSettings={() => setShowAppSettings(true)}
              onDevToolbar={devToolbarEnabled ? toggleDevToolbar : null}
              suggestRepoLink={!!suggestRepoLink}
            />
          </div>
        ) : (
          <div className="ml-auto flex flex-wrap items-center justify-end gap-2">
            <SessionCounter />
            <Button
              variant="neutral"
              size="icon"
              className="gap-1 lg:w-auto lg:px-2 lg:text-xs"
              onClick={() => setCreateBoardOpen(true)}
              aria-label="New board"
              title="New board"
            >
              <PlusIcon />
              <span className="hidden lg:inline">new board</span>
            </Button>
            {view === "board" && activeId != null && (
              <Button
                variant="neutral"
                size="icon"
                className="gap-1 lg:w-auto lg:px-2 lg:text-xs"
                onClick={() => setShowArchived(true)}
                aria-label="Archived tickets"
                title="Archived tickets"
              >
                <ArchiveIcon />
                <span className="hidden lg:inline">archived</span>
              </Button>
            )}
            <a
              href="https://jamison.lahman.dev/agentic-kanban/guide/"
              target="_blank"
              rel="noopener noreferrer"
              aria-label="Help"
              title="Help"
              className="rounded cursor-pointer transition-colors duration-150 bg-surface-2 text-fg hover:bg-surface-3 inline-flex h-7 w-7 items-center justify-center"
            >
              <HelpIcon />
            </a>
            {view === "board" && activeBoard && (
              <span className="relative inline-flex">
                <Button
                  variant="neutral"
                  size="icon"
                  onClick={() => setShowSettings(true)}
                  aria-label="Board settings"
                  title={
                    suggestRepoLink
                      ? "Board settings — git repo detected, link it for branches and pull requests"
                      : "Board settings"
                  }
                >
                  <CogIcon />
                </Button>
                {suggestRepoLink && (
                  <span
                    aria-hidden
                    className="pointer-events-none absolute -right-0.5 -top-0.5 h-2 w-2 rounded-full bg-accent-500 ring-2 ring-bg"
                  />
                )}
              </span>
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
            {devToolbarEnabled && (
              <Button
                variant={devToolbarPrefs.open ? "primary" : "neutral"}
                size="icon"
                onClick={toggleDevToolbar}
                aria-label="Developer toolbar"
                aria-pressed={devToolbarPrefs.open}
                title="Developer toolbar"
              >
                <GaugeIcon />
              </Button>
            )}
          </div>
        )}
      </header>
      {view === "board" && activeId != null && showStreamError && (
        <div className="border-b border-amber-700 bg-amber-950/60 px-4 py-1 text-xs text-amber-200">
          Live updates disconnected — reconnecting…
        </div>
      )}
      <main className="min-h-0 flex-1 overflow-hidden">
        {view === "overview" ? (
          <Overview />
        ) : activeId != null ? (
          <Board boardId={activeId} />
        ) : noBoards ? (
          <div className="flex h-full items-center justify-center p-4 text-sm text-fg-muted">
            No board selected.
          </div>
        ) : null}
      </main>
      <TerminalsRoot />
      {view === "board" && activeId != null && showArchived && (
        <ArchivedDrawer open boardId={activeId} onClose={() => setShowArchived(false)} />
      )}
      {view === "board" && activeBoard && showSettings && (
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
      <CreateBoardModal
        open={createBoardOpen}
        onClose={() => setCreateBoardOpen(false)}
        onCreated={onBoardCreated}
      />
      {devToolbarEnabled && devToolbarPrefs.open && <DevToolbar />}
    </div>
  );
}
