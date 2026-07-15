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
│   ├── done           Move a ticket to the rightmost column and stop its session
│   ├── sync           Sync a ticket branch from base
│   └── merge          Merge a ticket branch into base
├── column             Column-level operations
│   └── archive-all    Archive every ticket in a column
├── session            Manage agent sessions
│   ├── ensure         Ensure a session exists for a ticket
│   ├── start          Start a stopped session
│   ├── stop           Stop a running session
│   └── restart        Restart a session
├── config             Read and write kanban configuration
│   ├── list           List config keys with value and source
│   ├── get            Print one config value
│   ├── set            Set a config key
│   └── unset          Remove a config key
└── env                Manage board environment variables (write-only secrets)
    ├── list           List env var key names on a board
    ├── set            Set env vars on a board
    └── unset          Remove env vars from a board
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
| `--claude-config`    | `true`                                        | Forward host Claude Code config (`~/.claude`, `~/.claude.json`) into built-in session containers. When set explicitly (`--claude-config=false`), overrides `.kanban.toml [devcontainer].claude_config`; otherwise the toml setting wins. Env: `$KANBAN_CLAUDE_CONFIG`.                  |

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
| `--base-branch`   | no       | Branch new session worktrees fork from. Defaults to `main`. Before creating each worktree, kanban best-effort runs `git fetch origin <base-branch>` (10s timeout) and uses `origin/<base-branch>` as the start-point if it ends up strictly ahead of local; otherwise it falls back to local. |
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

Moves the ticket to the board's rightmost column and stops its session.

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

## `config`

Read and write the layered kanban config (see [Configuration](/guide/configuration)), mirroring `git config`. Keys are dotted, e.g. `sync.allow_rebase` or `github.draft_column`.

Scope flags are shared by every subcommand:

- `--global` — the user config (`~/.config/kanban/config.toml`).
- `--local --board <id-or-slug>` — that board's project `.kanban.toml`.
- Reads (`list`, `get`) default to the merged **effective** view when no scope flag is given; writes (`set`, `unset`) require `--global` or `--local`.

### `config list [--local | --global] [--board <id-or-slug>] [--json]`

Prints a `KEY  VALUE  SOURCE` table. The effective view also includes read-only runtime values (`data_dir`, ports) with source `runtime`.

### `config get <key> [--local | --global] [--board <id-or-slug>] [--json]`

Prints the bare value (so it's pipe-friendly). `--json` prints `{"key","value"}`. A single `devcontainer.container_env` entry is addressable as `devcontainer.container_env.<NAME>`.

### `config set <key> <value> (--global | --local --board <id-or-slug>)`

```sh
kanban config set sync.allow_rebase true --global
kanban config set github.draft_column "Draft" --global
kanban config set branches.prefix feat --local --board playground
# Collections take a JSON value:
kanban config set devcontainer.run_args '["--cap-add=NET_ADMIN"]' --global
kanban config set task '[{"label":"web","container_port":3000}]' --global
kanban config set devcontainer.container_env.FOO bar --global
```

Booleans accept `true`/`false`; strings pass through; arrays/maps/tables take a JSON document.

### `config unset <key> (--global | --local --board <id-or-slug>)`

Removes the key (and prunes any section it leaves empty).

## `env`

Manage per-board environment variables, injected into the board's session
containers at the next session start/restart. Values are write-only secrets:
they're encrypted at rest and no subcommand can print one — only key names
ever come back. See
[per-board environment variables](/guide/configuration#per-board-environment-variables).

### `env list <board>`

Prints the key names as a table. `<board>` is an id or slug.

### `env set <board> KEY=VALUE [KEY=VALUE...]`

```sh
kanban env set playground MY_API_KEY=sk-abc123 OTHER_TOKEN=xyz
```

Sets (or overwrites) variables. Keys must match `[A-Za-z_][A-Za-z0-9_]*`; the
`KANBAN_` prefix is reserved for server-injected variables.

### `env unset <board> KEY [KEY...]`

Removes variables by key name. Removing a missing key is a no-op.

## Environment variables

| Variable               | Used by                                       | Notes                                                           |
| ---------------------- | --------------------------------------------- | --------------------------------------------------------------- |
| `KANBAN_URL`           | `mcp`, `board`, `ticket`, `column`, `session`, `config`, `env` | Default server URL. Overridden by `--server` if explicitly set. |
| `KANBAN_CONFIG`        | `serve`                                       | User-level config path. Overridden by `--config` if set.        |
| `KANBAN_DATA_DIR`      | `serve`                                       | Data directory. Overridden by `--data-dir` if set.              |
| `KANBAN_WORKTREES_DIR` | `serve`                                       | Worktrees directory. Overridden by `--worktrees-dir` if set.    |
| `KANBAN_CLAUDE_CONFIG` | `serve`                                       | Forces built-in `claude_config` forwarding on/off (parsed as bool). Overridden by `--claude-config` if explicitly set. Either source wins over `.kanban.toml`. |
