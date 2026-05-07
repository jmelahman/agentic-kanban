# REST API

The HTTP server (default `:7474`) exposes a JSON REST API. The full route table lives in [`internal/api/api.go`](https://github.com/jmelahman/agentic-kanban/blob/master/internal/api/api.go); this page documents the endpoints most useful for scripting kanban from outside the UI.

::: warning No authentication
The server has no authentication. Bind only to `127.0.0.1` (the default container mapping does this).
:::

## Conventions

- All bodies are JSON unless noted.
- Path parameter `{id}` accepts either the numeric ID or the slug.
- Errors return `{ "error": "<message>" }` with a `4xx`/`5xx` status.

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

Create a new board. Body fields include `name`, `slug`, and `repo_path` (the local git repo the board tracks).

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
- `task_run_*`

Useful for keeping a dashboard or another integration in sync without polling.

## Sessions

Sessions are the running container + worktree + harness for a ticket. The most useful endpoints:

- `POST /api/tickets/{id}/session` — ensure a session exists for the ticket.
- `POST /api/sessions/{id}/start` / `stop` / `restart`
- `PATCH /api/sessions/{id}/status` — used by the harness to report state changes.
- `GET /api/sessions/{id}/ports` — list active port proxies.

## Health & metadata

- `GET /api/health` — returns `200 OK` once the server is ready.
- `GET /api/version` — returns `{ "version": "<semver-or-sha>" }`.

## Frontend error reporting

- `POST /api/errors` — used internally by the web UI to file frontend exceptions as tickets when error reporting is enabled.
