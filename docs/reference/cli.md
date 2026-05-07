# CLI

The same `kanban` binary serves the HTTP server, runs the MCP server, and provides two convenience subcommands for scripting against a running server.

```text
kanban
├── serve            Start the kanban HTTP server
├── mcp              Run kanban as an MCP server over stdio
├── list-boards      List boards as a table
└── create-ticket    Create a ticket on a board
```

`kanban --version` prints the build version.

## `serve`

Starts the HTTP server.

```sh
kanban serve [flags]
```

| Flag                  | Default                                | Description                                                                                                                              |
|-----------------------|----------------------------------------|------------------------------------------------------------------------------------------------------------------------------------------|
| `--addr`              | `:7474`                                | HTTP listen address.                                                                                                                     |
| `--data-dir`          | `$KANBAN_DATA_DIR` or XDG share dir    | Override the data directory (SQLite + worktrees).                                                                                        |
| `--worktrees-dir`     | `$KANBAN_WORKTREES_DIR` or `<data>/worktrees` | Override where new worktrees are created.                                                                                          |
| `--config`            | `$KANBAN_CONFIG` or XDG config dir     | Override the user-level kanban config path.                                                                                              |
| `--port-range-start`  | `13000`                                | First host port available for proxy allocation.                                                                                          |
| `--port-range-end`    | `13099`                                | Last host port available for proxy allocation (inclusive).                                                                               |
| `--in-memory`         | `false`                                | Dev/test only. Use an ephemeral in-memory SQLite database; **all data is discarded on shutdown**. The server logs `WARNING: --in-memory set` at startup. |

## `mcp`

Runs kanban as a [Model Context Protocol](https://modelcontextprotocol.io) server over stdio. The MCP server is a thin client of the HTTP API, so a separate `kanban serve` must be running.

```sh
kanban mcp [--server URL]
# or, equivalently:
KANBAN_URL=http://localhost:7474 kanban mcp
```

| Flag       | Default                  | Description                                |
|------------|--------------------------|--------------------------------------------|
| `--server` | `http://localhost:7474`  | Base URL of the kanban HTTP server.        |

See the [MCP reference](./mcp) for tool definitions and Claude Desktop / Claude Code wiring.

## `list-boards`

```sh
kanban list-boards [--server URL]
```

Prints all boards as a `id slug name` table.

## `create-ticket`

```sh
kanban create-ticket --board <id-or-slug> --title <title> [flags]
```

| Flag       | Required | Default                  | Description                                                              |
|------------|----------|--------------------------|--------------------------------------------------------------------------|
| `--board`  | yes      |                          | Board id or slug.                                                        |
| `--title`  | yes      |                          | Ticket title.                                                            |
| `--body`   | no       |                          | Markdown ticket body.                                                    |
| `--column` | no       | leftmost column          | Column name (case-insensitive) or numeric id.                            |
| `--json`   | no       | `false`                  | Print the full ticket JSON instead of a one-line summary.                |
| `--server` | no       | `http://localhost:7474`  | Base URL of the kanban HTTP server. Falls back to `$KANBAN_URL`.         |

```sh
# Default column (leftmost)
kanban create-ticket --board playground --title "Investigate flaky test"

# Specific column, with body, full JSON output
kanban create-ticket \
  --board playground \
  --title "Wire CI" \
  --column "In Progress" \
  --body "Add a docs workflow that builds the site on push to master." \
  --json
```

## Environment variables

| Variable                  | Used by                | Notes                                                                  |
|---------------------------|------------------------|------------------------------------------------------------------------|
| `KANBAN_URL`              | `mcp`, `list-boards`, `create-ticket` | Default server URL. Overridden by `--server` if explicitly set.       |
| `KANBAN_CONFIG`           | `serve`                | User-level config path. Overridden by `--config` if set.                |
| `KANBAN_DATA_DIR`         | `serve`                | Data directory. Overridden by `--data-dir` if set.                      |
| `KANBAN_WORKTREES_DIR`    | `serve`                | Worktrees directory. Overridden by `--worktrees-dir` if set.            |
