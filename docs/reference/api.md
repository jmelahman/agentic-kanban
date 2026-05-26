# REST API

The HTTP server (default `:7474`) exposes a JSON REST API. The full route table lives in [`internal/api/api.go`](https://github.com/jmelahman/agentic-kanban/blob/master/internal/api/api.go); this page documents the endpoints most useful for scripting kanban from outside the UI.

::: warning No authentication
The server has no authentication. Bind only to `127.0.0.1` (the default container mapping does this).
:::

## Conventions

- All bodies are JSON unless noted.
- Path parameter `{id}` accepts either the numeric ID or the slug.
- Errors return `{ "error": "<message>" }` with a `4xx`/`5xx` status.
- Responses ≥1 KB are gzip-compressed when the client sends `Accept-Encoding: gzip`. Send `Accept-Encoding: identity` to opt out. SSE streams are never compressed.

## Tickets

### `POST /api/boards/{id}/tickets`

Create a ticket on the given board.

| Field       | Type    | Required | Notes                                                                                           |
|-------------|---------|----------|-------------------------------------------------------------------------------------------------|
| `title`     | string  | yes      |                                                                                                 |
| `body`      | string  | no       | Markdown ticket description.                                                                    |
| `column_id` | integer | no       | Numeric column id. Wins over `column` if both set.                                              |
| `column`    | string  | no       | Column name (case-insensitive) or numeric string. Defaults to the leftmost column when omitted. |

Returns `201` with the created `Ticket` JSON, or `400` / `404` on validation failures.

```sh
curl -fsS -X POST http://localhost:7474/api/boards/playground/tickets \
  -H 'Content-Type: application/json' \
  -d '{"title":"Investigate flaky test","column":"Backlog","body":"Repro on CI but not locally."}'
```

### `GET /api/boards/{id}/archived`

List tickets that have been archived from the given board.

### `PATCH /api/tickets/{id}`

Update a ticket's `title` or `body`. Send only the fields you want to change.

### `PATCH /api/tickets/{id}/move`

Move a ticket to a different column or position. Body: `{"column_id": <id>, "position": <int>}`.

### `POST /api/tickets/{id}/archive` / `unarchive`

Move a ticket out of (or back into) the active board.

### `DELETE /api/tickets/{id}`

Permanently remove a ticket. Cleans up its session and worktree.

## Boards

### `GET /api/boards`

List all boards.

### `POST /api/boards`

Create a new board. Body fields:

| Field | Type | Notes |
| --- | --- | --- |
| `name` | string | Required. Display name; the slug is derived from this. |
| `repo_path` | string | Host path to the git repo. One of `repo_path` / `mount_path` is required. |
| `mount_path` | string | Host directory mounted into session containers, when distinct from the repo. |
| `worktree_root` | string | Parent directory for new session worktrees. Defaults to `<data_dir>/worktrees/<slug>`. |
| `base_branch` | string | Branch session worktrees fork from. When omitted, the server detects it from `repo_path` (`origin/HEAD`, falling back to the currently checked-out branch); if detection fails it defaults to `main`. Each new session worktree best-effort fetches `origin/<base_branch>` (10s timeout) and uses it as the start-point if it ends up strictly ahead of local; on failure, tie, or divergence, the local branch is used. |
| `branch_prefix` | string | Prefix prepended to session branch names. |
| `git_author_name` | string | Optional. Author name used for merge/squash commits the server creates. |
| `git_author_email` | string | Optional. Author email used for merge/squash commits. |

If both `git_author_name` and `git_author_email` are set, the server passes `-c user.name=… -c user.email=…` to `git commit` when finishing a session — needed when kanban runs in a container without a configured git identity. Leave both blank to fall back to whatever the kanban container's `git config` resolves.

### `PATCH /api/boards/{id}`

Patch any of the fields above. Pointer-style: omit a field to leave it untouched, send an empty string to clear it.

### `PATCH /api/boards/{id}/move`

Reorder boards. Body: `{"position": <int>}`, where `position` is the zero-based index the board should occupy in `GET /api/boards` after the move. The server renumbers every board so positions stay contiguous. `GET /api/boards` orders by this field, and the Overview sidebar exposes it as drag-to-reorder.

### `GET /api/boards/{id}`

Fetch a single board.

### `GET /api/boards/{id}/state`

Get the full hydrated board state — columns, tickets, sessions — in one call. The web UI uses this on load.

## Live updates (SSE)

### `GET /api/boards/{id}/events`

Server-Sent Events stream of board changes. Events include:

- `ticket_created`
- `ticket_updated`
- `ticket_moved`
- `ticket_archived`
- `session_status_changed`
- `session_pull_progress` — emitted while a session is starting and its
  devcontainer image is being pulled. The payload is
  `{ session_id, image, current, total, layers, status, done }` where
  `current`/`total` are aggregate bytes summed across layers (`total` is `0`
  until the daemon reports the first byte counter), `status` is the most
  recent human-readable line from Docker (e.g. `Downloading`,
  `Extracting`), and `done` is `true` on the final terminal event.
- `task_run_*`

Useful for keeping a dashboard or another integration in sync without polling.

## Sessions

Sessions are the running container + worktree + harness for a ticket. The most useful endpoints:

- `POST /api/tickets/{id}/session` — ensure a session exists for the ticket.
- `POST /api/sessions/{id}/start` / `stop` / `restart`
- `PATCH /api/sessions/{id}/status` — used by the harness to report state changes.
- `PATCH /api/sessions/{id}/claude-session` — called by the Claude Code `SessionStart` hook with `{ "claude_session_id": "<uuid>" }` so kanban can `--resume` the same conversation on the next launch after a container/Kanban restart. Rejects non-UUID values with `400`. Clearing the stored UUID (to force a fresh conversation) is a manual `UPDATE sessions SET claude_session_id = NULL` on the DB.
- `GET /api/sessions/{id}/ports` — list active port proxies.
- `GET /api/sessions/{id}/pr-detail` — live snapshot of the linked GitHub PR: diff totals, rolled-up review decision, and check status (passed / failing / pending counts plus the names of failing checks). Hits the GitHub API on each call; returns `404` if the session has no PR and `502` if GitHub is unreachable. The session payload itself carries `pr_state`, `pr_number`, `pr_url`, and `pr_title` (refreshed by the GitHub poller).

## Filesystem

- `GET /api/fs/check?path=<path>` — returns `{ "state": "git" | "not_git" | "unknown" }` for the given path as seen from inside the kanban server. `git` means the path is a directory containing `.git`; `not_git` means it exists but isn't a repo; `unknown` means it isn't visible to the kanban container (it may still be a valid host path that dockerd can mount when a session spawns — kanban just can't `stat` it). Used by the UI to badge the board-settings icon when a board's mount path looks like a git repo but no `repo_path` is configured.

## Health & metadata

- `GET /api/health` — returns `200 OK` with `{"status":"ok"}` when both the SQLite database and the Docker daemon are reachable. Returns `503 Service Unavailable` with `{"db":"..."}` or `{"docker":"..."}` otherwise. Used by the container `HEALTHCHECK` directive in the published image so orchestrators can detect a wedged process.
- `GET /api/version` — returns `{ "version": "<semver-or-sha>" }`.
- `GET /metrics` — Prometheus exposition format. Lists Go runtime/process metrics plus kanban-specific series: `kanban_http_requests_total`, `kanban_http_request_duration_seconds`, `kanban_db_query_duration_seconds`, `kanban_db_query_errors_total`, `kanban_github_api_requests_total`, `kanban_github_api_request_duration_seconds`, `kanban_github_rate_limit_remaining`, `kanban_github_rate_limit_limit`, and `kanban_github_rate_limit_reset_timestamp_seconds`. See [observability](../guide/observability) for the bundled Prometheus service that scrapes this endpoint.
- `GET /prometheus/...` — reverse-proxy to the Prometheus UI when `KANBAN_PROMETHEUS_URL` is set. Used by the bundled `prometheus` compose service; returns 503 in dev runs without Prometheus.

## Frontend error reporting

- `POST /api/errors` — used internally by the web UI to file frontend exceptions
  as tickets when error reporting is enabled. The body is
  `{ message, stack, source, url, user_agent, meta }`; `meta` is an optional
  string map merged into the ticket body. Sources used today:
  `boundary` (React render errors), `react-query` (query/mutation 5xx),
  `window` (uncaught exceptions), `unhandledrejection` (rejected promises with
  no `.catch`), and `longtask` (main-thread blocks ≥2s reported by the
  PerformanceObserver — useful for catching UI freezes that throw nothing).
  Always returns 204. Every accepted report is also written to the server's
  stdout logger (one `client error: source=… url=… message=…` line plus the
  stack), so frontend errors show up in `docker logs` even when
  `[errors] enabled` is false; ticket creation is the only path gated by that
  flag.
