CREATE TABLE IF NOT EXISTS boards (
  id INTEGER PRIMARY KEY,
  name TEXT NOT NULL,
  slug TEXT NOT NULL UNIQUE,
  repo_path TEXT,
  mount_path TEXT,
  worktree_root TEXT,
  base_branch TEXT NOT NULL DEFAULT 'main',
  branch_prefix TEXT,
  created_at INTEGER NOT NULL DEFAULT (unixepoch())
);

CREATE TABLE IF NOT EXISTS columns (
  id INTEGER PRIMARY KEY,
  board_id INTEGER NOT NULL REFERENCES boards(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  position INTEGER NOT NULL,
  UNIQUE(board_id, position)
);

CREATE TABLE IF NOT EXISTS tickets (
  id INTEGER PRIMARY KEY,
  board_id INTEGER NOT NULL REFERENCES boards(id) ON DELETE CASCADE,
  column_id INTEGER NOT NULL REFERENCES columns(id) ON DELETE CASCADE,
  title TEXT NOT NULL,
  slug TEXT NOT NULL,
  body TEXT NOT NULL DEFAULT '',
  position INTEGER NOT NULL,
  created_at INTEGER NOT NULL DEFAULT (unixepoch()),
  archived_at INTEGER,
  fingerprint TEXT,
  UNIQUE(board_id, slug)
);

CREATE TABLE IF NOT EXISTS sessions (
  id INTEGER PRIMARY KEY,
  ticket_id INTEGER NOT NULL UNIQUE REFERENCES tickets(id) ON DELETE CASCADE,
  worktree_path TEXT NOT NULL,
  branch_name TEXT NOT NULL,
  container_id TEXT,
  container_name TEXT,
  status TEXT NOT NULL DEFAULT 'stopped',
  started_at INTEGER,
  stopped_at INTEGER,
  pr_state TEXT NOT NULL DEFAULT '',
  pr_number INTEGER,
  pr_url TEXT,
  mount_path TEXT,
  repo_path TEXT
);

CREATE TABLE IF NOT EXISTS port_allocations (
  id INTEGER PRIMARY KEY,
  session_id INTEGER NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
  label TEXT NOT NULL,
  container_port INTEGER NOT NULL,
  host_port INTEGER NOT NULL UNIQUE,
  proxy_active INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS task_runs (
  id INTEGER PRIMARY KEY,
  session_id INTEGER NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
  task_label TEXT NOT NULL,
  command TEXT NOT NULL,
  exec_id TEXT,
  status TEXT NOT NULL DEFAULT 'running',
  exit_code INTEGER,
  started_at INTEGER NOT NULL DEFAULT (unixepoch()),
  stopped_at INTEGER
);

CREATE TABLE IF NOT EXISTS hook_configs (
  id INTEGER PRIMARY KEY,
  board_id INTEGER REFERENCES boards(id) ON DELETE CASCADE,
  event TEXT NOT NULL,
  command TEXT NOT NULL,
  enabled INTEGER NOT NULL DEFAULT 1
);

CREATE INDEX IF NOT EXISTS idx_tickets_column_pos
  ON tickets(column_id, position) WHERE archived_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_tickets_board_archived
  ON tickets(board_id, archived_at);
CREATE INDEX IF NOT EXISTS idx_task_runs_session
  ON task_runs(session_id, id DESC);
CREATE INDEX IF NOT EXISTS idx_hook_configs_event
  ON hook_configs(event, enabled, board_id);
CREATE INDEX IF NOT EXISTS idx_port_alloc_session
  ON port_allocations(session_id);
