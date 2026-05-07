export type Board = {
  id: number;
  name: string;
  slug: string;
  repo_path: string;
  mount_path: string;
  worktree_root: string;
  base_branch: string;
  branch_prefix: string;
  created_at: number;
};

export type Column = { id: number; board_id: number; name: string; position: number };

export type Ticket = {
  id: number;
  board_id: number;
  column_id: number;
  title: string;
  slug: string;
  body: string;
  position: number;
  created_at: number;
  archived_at?: number;
};

export type PRState = "draft" | "open" | "merged" | "closed";

export const PR_STATE_COLOR: Record<PRState, string> = {
  draft: "text-zinc-400",
  open: "text-emerald-400",
  merged: "text-purple-400",
  closed: "text-red-400",
};

export type Session = {
  id: number;
  ticket_id: number;
  worktree_path: string;
  branch_name: string;
  container_id?: string;
  container_name?: string;
  status: string;
  started_at?: number;
  stopped_at?: number;
  pr_state?: PRState | "";
  pr_number?: number;
  pr_url?: string;
};

export type MergeConfig = {
  allow_merge_commit: boolean;
  allow_squash: boolean;
  allow_rebase: boolean;
};

export type SyncConfig = {
  allow_rebase: boolean;
  allow_merge: boolean;
};

export type BoardState = {
  board: Board;
  columns: Column[];
  tickets: Ticket[];
  sessions: Session[];
  merge_config: MergeConfig;
  sync_config: SyncConfig;
};

export type DiscoveredTask = {
  label: string;
  command: string;
  args: string[];
  cwd: string;
  env: Record<string, string>;
  container_port?: number;
  has_port: boolean;
};

export type DiscoverTasksResult = {
  tasks: DiscoveredTask[];
  warnings: string[];
};

export type TaskRun = {
  id: number;
  session_id: number;
  task_label: string;
  command: string;
  status: string;
  exit_code?: number;
  started_at: number;
  stopped_at?: number;
};

export type PortAllocation = {
  id: number;
  session_id: number;
  label: string;
  container_port: number;
  host_port: number;
  proxy_active: boolean;
};

export type AppSettings = {
  harness: string;
  worktrees_root: string;
  worktrees_root_resolved: string;
  worktrees_root_locked: boolean;
};

export type Harness = { id: string; label: string; pty_command: string[] };

export type Version = { version: string };

export class ApiError extends Error {
  status: number;
  body: string;
  constructor(status: number, message: string, body: string) {
    super(message);
    this.name = "ApiError";
    this.status = status;
    this.body = body;
  }
}

// Error-handling convention: mutations should not call toast.push("error", ...)
// for the default case. The QueryClient in main.tsx is configured with a global
// onError handler that toasts every mutation/query failure via this helper.
// Per-mutation onError is reserved for behaviors beyond toasting (closing a
// menu, rolling back optimistic state, resetting a ref, etc.).
export function formatApiError(err: unknown): string {
  if (err instanceof ApiError) return `${err.status}: ${err.message}`;
  if (err instanceof Error) return err.message;
  return String(err);
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(path, {
    headers: { "Content-Type": "application/json", ...(init?.headers ?? {}) },
    ...init,
  });
  if (!res.ok) {
    const text = await res.text();
    let message = text;
    try {
      const parsed = JSON.parse(text);
      if (parsed && typeof parsed.error === "string") message = parsed.error;
    } catch {
      // not JSON; use raw body
    }
    throw new ApiError(res.status, message || `HTTP ${res.status}`, text);
  }
  if (res.status === 204) return undefined as T;
  return res.json();
}

export const api = {
  listBoards: () => request<Board[]>("/api/boards"),
  createBoard: (input: { name: string; repo_path?: string; mount_path?: string; worktree_root?: string; base_branch?: string; branch_prefix?: string }) =>
    request<Board>("/api/boards", { method: "POST", body: JSON.stringify(input) }),
  updateBoard: (id: number, input: { name?: string; repo_path?: string; mount_path?: string; worktree_root?: string; base_branch?: string; branch_prefix?: string }) =>
    request<Board>(`/api/boards/${id}`, { method: "PATCH", body: JSON.stringify(input) }),
  deleteBoard: (id: number) => request<void>(`/api/boards/${id}`, { method: "DELETE" }),
  boardState: (id: number) => request<BoardState>(`/api/boards/${id}/state`),

  createTicket: (boardId: number, input: { column_id: number; title: string; body?: string }) =>
    request<Ticket>(`/api/boards/${boardId}/tickets`, { method: "POST", body: JSON.stringify(input) }),
  updateTicket: (id: number, input: { title?: string; body?: string }) =>
    request<Ticket>(`/api/tickets/${id}`, { method: "PATCH", body: JSON.stringify(input) }),
  moveTicket: (id: number, input: { column_id: number; position: number }) =>
    request<void>(`/api/tickets/${id}/move`, { method: "PATCH", body: JSON.stringify(input) }),
  archiveTicket: (id: number) => request<void>(`/api/tickets/${id}/archive`, { method: "POST" }),
  unarchiveTicket: (id: number) => request<void>(`/api/tickets/${id}/unarchive`, { method: "POST" }),
  listArchivedTickets: (boardId: number) => request<Ticket[]>(`/api/boards/${boardId}/archived`),
  deleteTicket: (id: number) => request<void>(`/api/tickets/${id}`, { method: "DELETE" }),
  syncTicket: (id: number, strategy: "rebase" | "merge") =>
    request<void>(`/api/tickets/${id}/sync`, { method: "POST", body: JSON.stringify({ strategy }) }),
  mergeTicket: (id: number, strategy: "merge-commit" | "squash" | "rebase") =>
    request<void>(`/api/tickets/${id}/merge`, { method: "POST", body: JSON.stringify({ strategy }) }),
  doneTicket: (id: number) => request<void>(`/api/tickets/${id}/done`, { method: "POST" }),

  ensureSession: (ticketId: number) => request<Session>(`/api/tickets/${ticketId}/session`, { method: "POST" }),
  startSession: (id: number) => request<Session>(`/api/sessions/${id}/start`, { method: "POST" }),
  stopSession: (id: number) => request<void>(`/api/sessions/${id}/stop`, { method: "POST" }),
  restartSession: (id: number) => request<Session>(`/api/sessions/${id}/restart`, { method: "POST" }),

  discoverTasks: (sessionId: number) => request<DiscoverTasksResult>(`/api/sessions/${sessionId}/discover-tasks`),
  listTaskRuns: (sessionId: number) => request<TaskRun[]>(`/api/sessions/${sessionId}/task-runs`),
  startTaskRun: (sessionId: number, label: string) =>
    request<TaskRun>(`/api/sessions/${sessionId}/task-runs`, { method: "POST", body: JSON.stringify({ label }) }),
  stopTaskRun: (id: number) => request<void>(`/api/task-runs/${id}`, { method: "DELETE" }),

  listPorts: (sessionId: number) => request<PortAllocation[]>(`/api/sessions/${sessionId}/ports`),
  createPort: (sessionId: number, input: { label: string; container_port: number }) =>
    request<PortAllocation[]>(`/api/sessions/${sessionId}/ports`, { method: "POST", body: JSON.stringify(input) }),
  deletePort: (id: number) => request<void>(`/api/ports/${id}`, { method: "DELETE" }),

  getSettings: () => request<AppSettings>("/api/settings"),
  updateSettings: (input: { harness?: string; worktrees_root?: string }) =>
    request<AppSettings>("/api/settings", { method: "PATCH", body: JSON.stringify(input) }),
  listHarnesses: () => request<Harness[]>("/api/harnesses"),

  getVersion: () => request<Version>("/api/version"),
};

export type SubscribeOptions = {
  onEvent: (type: string, data: unknown) => void;
  onStatus?: (status: "open" | "error" | "closed") => void;
};

// postError reports a frontend exception to the backend, which may file a
// kanban ticket if error reporting is enabled in `.kanban.toml`. Always
// best-effort: failures are silently swallowed, and a single in-flight call
// blocks re-entry so one capture site can never spiral into a loop.
//
// Uses raw fetch (not request<T>) so an HTTP error here can't throw into the
// React Query global onError and re-trigger this function.
let reportingError = false;
export async function postError(p: {
  message: string;
  stack?: string;
  source: string;
  url?: string;
  user_agent?: string;
}): Promise<void> {
  if (reportingError) return;
  reportingError = true;
  try {
    await fetch("/api/errors", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        message: p.message,
        stack: p.stack ?? "",
        source: p.source,
        url: p.url ?? (typeof window !== "undefined" ? window.location.href : ""),
        user_agent: p.user_agent ?? (typeof navigator !== "undefined" ? navigator.userAgent : ""),
      }),
    });
  } catch {
    // never toast, never recurse
  } finally {
    reportingError = false;
  }
}

export function subscribeBoard(boardId: number, opts: SubscribeOptions): () => void {
  const es = new EventSource(`/api/boards/${boardId}/events`);
  // The backend wraps each event as {type, data}; unwrap so consumers see the
  // raw entity (Ticket / Session / …). Without this, `data.id` is undefined
  // and per-id stores never update — selections appear stuck (e.g. a session
  // pinned at "starting" forever).
  const handler = (e: MessageEvent) => {
    try {
      const parsed = JSON.parse(e.data);
      const payload =
        parsed && typeof parsed === "object" && "data" in parsed
          ? parsed.data
          : parsed;
      opts.onEvent(e.type, payload);
    } catch {
      opts.onEvent(e.type, null);
    }
  };
  for (const t of ["ticket_created", "ticket_updated", "ticket_moved", "ticket_archived", "ticket_unarchived", "ticket_deleted", "session_updated", "ready"]) {
    es.addEventListener(t, handler as EventListener);
  }
  es.onopen = () => opts.onStatus?.("open");
  es.onerror = () => opts.onStatus?.("error");
  return () => {
    es.close();
    opts.onStatus?.("closed");
  };
}
