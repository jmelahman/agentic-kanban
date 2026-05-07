# CLI

The same `kanban` binary serves the HTTP server, runs the MCP server, and
provides noun-verb subcommands for scripting against a running server.

```text
kanban
├── serve              Start the kanban HTTP server
├── mcp                Run kanban as an MCP server over stdio
├── board              Manage kanban boards
│   ├── list           List boards as a table
│   ├── create         Create a new board
│   ├── get            Print a single board
│   ├── update         Update fields on a board
│   ├── delete         Delete a board (and destroy all its sessions)
│   ├── state          Print full board state as JSON
│   ├── archived       List archived tickets on a board
│   └── archived-clear Permanently delete every archived ticket on a board
├── ticket             Manage tickets on a board
│   ├── create         Create a ticket
│   ├── update         Update title/body of a ticket
│   ├── move           Move a ticket to a different column / position
│   ├── archive        Archive a ticket
│   ├── unarchive      Unarchive a ticket
│   ├── delete         Permanently delete (must be archived first)
│   ├── done           Move a ticket to the Done column and stop its session
│   ├── sync           Sync a ticket branch from base
│   └── merge          Merge a ticket branch into base
├── column             Column-level operations
│   └── archive-all    Archive every ticket in a column
└── session            Manage agent sessions
    ├── ensure         Ensure a session exists for a ticket
    ├── start          Start a stopped session
    ├── stop           Stop a running session
    └── restart        Restart a session
```

`kanban --version` prints the build version.

## `serve`

Starts the HTTP server.

```sh
kanban serve [flags]
```

| Flag                 | Default                                       | Description                                                                                                                                              |
| -------------------- | --------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `--addr`             | `:7474`                                       | HTTP listen address.                                                                                                                                     |
| `--data-dir`         | `$KANBAN_DATA_DIR` or XDG share dir           | Override the data directory (SQLite + worktrees).                                                                                                        |
| `--worktrees-dir`    | `$KANBAN_WORKTREES_DIR` or `<data>/worktrees` | Override where new worktrees are created.                                                                                                                |
| `--config`           | `$KANBAN_CONFIG` or XDG config dir            | Override the user-level kanban config path.                                                                                                              |
| `--port-range-start` | `13000`                                       | First host port available for proxy allocation.                                                                                                          |
| `--port-range-end`   | `13099`                                       | Last host port available for proxy allocation (inclusive).                                                                                               |
| `--in-memory`        | `false`                                       | Dev/test only. Use an ephemeral in-memory SQLite database; **all data is discarded on shutdown**. The server logs `WARNING: --in-memory set` at startup. |

## `mcp`

Runs kanban as a [Model Context Protocol](https://modelcontextprotocol.io)
server over stdio. The MCP server is a thin client of the HTTP API, so a
separate `kanban serve` must be running.

```sh
kanban mcp [--server URL]
# or, equivalently:
KANBAN_URL=http://localhost:7474 kanban mcp
```

| Flag       | Default                 | Description                         |
| ---------- | ----------------------- | ----------------------------------- |
| `--server` | `http://localhost:7474` | Base URL of the kanban HTTP server. |

See the [MCP reference](./mcp) for tool definitions and Claude Desktop /
Claude Code wiring.

## Common flags

Every subcommand under `board`, `ticket`, `column`, and `session`
inherits a single `--server URL` flag (default `http://localhost:7474`,
overridden by `$KANBAN_URL` when `--server` is not explicitly set).
Read commands that return rich data accept `--json` to print the raw
API JSON instead of a one-line summary.

## `board`

### `board list`

```sh
kanban board list
```

Prints all boards as an `ID SLUG NAME` table.

### `board create`

```sh
kanban board create --name <name> --repo-path <path> [flags]
```

| Flag              | Required | Description                                                         |
| ----------------- | -------- | ------------------------------------------------------------------- |
| `--name`          | yes      | Board name.                                                         |
| `--repo-path`     | one of   | Path to the host git repo. Required if `--mount-path` is not set.   |
| `--mount-path`    | one of   | Mount path inside session containers. Alternative to `--repo-path`. |
| `--worktree-root` | no       | Override the parent directory for new session worktrees.            |
| `--base-branch`   | no       | Branch new session worktrees fork from. Defaults to `main`.         |
| `--branch-prefix` | no       | Optional prefix prepended to session branch names.                  |
| `--json`          | no       | Print the full board JSON instead of a one-line summary.            |

### `board get <id>`

Prints a single board. `<id>` accepts a numeric id or a slug.

### `board update <id> [flags]`

Patches the supplied fields. Any flag you omit is left untouched.
Same flags as `board create` (minus `--name`, which can also be passed
to rename the board).

### `board delete <id>`

Deletes a board and destroys every associated session.

### `board state <id>`

Prints a single-shot snapshot — `{board, columns, tickets, sessions,
merge_config, sync_config}` — as JSON. Useful piping into `jq`.

### `board archived <id>`

Lists archived tickets on a board as an `ID SLUG TITLE` table.

### `board archived-clear <id>`

Permanently deletes every archived ticket on the board. Destructive.

## `ticket`

### `ticket create`

```sh
kanban ticket create --board <id-or-slug> --title <title> [flags]
```

| Flag       | Required | Default         | Description                                               |
| ---------- | -------- | --------------- | --------------------------------------------------------- |
| `--board`  | yes      |                 | Board id or slug.                                         |
| `--title`  | yes      |                 | Ticket title.                                             |
| `--body`   | no       |                 | Markdown ticket body.                                     |
| `--column` | no       | leftmost column | Column name (case-insensitive) or numeric id.             |
| `--json`   | no       | `false`         | Print the full ticket JSON instead of a one-line summary. |

```sh
kanban ticket create --board playground --title "Wire CI" --column "In Progress"
```

### `ticket update <id> [flags]`

| Flag      | Description                 |
| --------- | --------------------------- |
| `--title` | New title.                  |
| `--body`  | New body.                   |
| `--json`  | Print the full ticket JSON. |

### `ticket move <id> --column-id <int> [--position <int>]`

Moves a ticket. `--column-id` is numeric and required (look up column
ids via `kanban board state <id>`).

### `ticket archive <id>` / `ticket unarchive <id>`

Archive or restore a ticket. Archiving stops any running session.

### `ticket delete <id>`

Permanently deletes a ticket. The ticket must be archived first
(`ticket archive <id>` then `ticket delete <id>`).

### `ticket done <id>`

Moves the ticket to the Done column and stops its session.

### `ticket sync <id> [--strategy rebase|merge]`

Sync a ticket branch from base. Strategy defaults to `rebase`.

### `ticket merge <id> --strategy merge-commit|squash|rebase`

Merge a ticket branch into base. Strategy is required.

## `column`

### `column archive-all <id>`

Archives every non-archived ticket in the column. `<id>` is the numeric
column id.

## `session`

### `session ensure --ticket <id> [--json]`

Ensures a session exists for the given ticket; creates one if absent.

### `session start <id> [--json]` / `session restart <id> [--json]`

Starts or restarts a session by numeric session id.

### `session stop <id>`

Stops a running session.

## Environment variables

| Variable               | Used by                                       | Notes                                                           |
| ---------------------- | --------------------------------------------- | --------------------------------------------------------------- |
| `KANBAN_URL`           | `mcp`, `board`, `ticket`, `column`, `session` | Default server URL. Overridden by `--server` if explicitly set. |
| `KANBAN_CONFIG`        | `serve`                                       | User-level config path. Overridden by `--config` if set.        |
| `KANBAN_DATA_DIR`      | `serve`                                       | Data directory. Overridden by `--data-dir` if set.              |
| `KANBAN_WORKTREES_DIR` | `serve`                                       | Worktrees directory. Overridden by `--worktrees-dir` if set.    |
