# Agentic Kanban

[![Deploy Status](https://github.com/jmelahman/agentic-kanban/actions/workflows/release.yml/badge.svg)](https://github.com/jmelahman/agentic-kanban/actions)
[![Go Reference](https://pkg.go.dev/badge/github.com/jmelahman/agentic-kanban.svg)](https://pkg.go.dev/github.com/jmelahman/agentic-kanban)
[![PyPI](https://img.shields.io/pypi/v/release-agentic-kanban.svg)](https://pypi.org/project/agentic-kanban/)

A kanban board for managing AI agent sessions.

<p align="center">
  <picture align="center">
    <source media="(prefers-color-scheme: dark)" srcset="https://github.com/jmelahman/agentic-kanban/blob/master/assets/demo_dark.png">
    <source media="(prefers-color-scheme: light)" srcset="https://github.com/jmelahman/agentic-kanban/blob/master/assets/demo_light.png">
    <img src="https://github.com/jmelahman/agentic-kanban/blob/master/assets/demo_light.png">
  </picture>
</p>

Each ticket is bound to an agent session (Claude Code, pi.dev) running inside its own git worktree, executed in the target repository's existing devcontainer.
The active harness is selected globally in the app's settings.

## Install

**python:**

```
uv tool install agentic-kanban
```

This will install the binary to `~/.local/bin/kanban`.

**docker:**

```bash
SOURCE=$HOME/code
docker run -d --name kanban \
  --restart unless-stopped \
  -p 127.0.0.1:7474:7474 \
  -p 13000-13099:13000-13099 \
  -v $XDG_RUNTIME_DIR/docker.sock:/var/run/docker.sock \
  -v $HOME/.claude:$HOME/.claude \
  -v $HOME/.local/share/kanban:$HOME/.local/share/kanban \
  -v $SOURCE:$SOURCE \
  -e HOME=$HOME \
  -e XDG_RUNTIME_DIR=$XDG_RUNTIME_DIR \
  -e KANBAN_DATA_DIR=$HOME/.local/share/kanban \
  -e GH_TOKEN=$(gh auth token) \
  lahmanja/kanban:latest
```

**github:**

Prebuilt packages are available from [Github Releases](https://github.com/jmelahman/agentic-kanban/releases).

## Build

```bash
docker bake
```

## Configuration

Kanban reads two TOML files and merges them, with user values overriding project values per key:

- **Project**: `<repo>/.kanban.toml` — checked into the target repo, applies to every worktree of that repo.
- **User**: `$XDG_CONFIG_HOME/kanban/config.toml` (falling back to `~/.config/kanban/config.toml`) — your personal overrides across all repos.

Either file may be absent. Both accept the same schema:

```toml
[harness]
id = "claude-code"            # default harness for new sessions

[sync]
allow_rebase = true            # offer "rebase onto base" in the sync menu
allow_merge  = true            # offer "merge base into branch"

[merge]
allow_merge_commit = true      # which strategies appear in the merge menu
allow_squash       = true
allow_rebase       = false

[github]
auto_move     = true           # move tickets when the linked PR/issue changes state
draft_column  = "In Progress"
review_column = "In Review"
done_column   = "Done"
closed_column = "Done"

# Per-task ports: associate .vscode/tasks.json labels with container ports.
# When such a task runs, kanban allocates a host port from 13000-13099 and
# runs a TCP proxy.
[[task]]
label = "Start Frontend"
container_port = 3000

[[task]]
label = "Start Backend"
container_port = 8080
```

`[[task]]` entries merge by `label`: a user entry with the same label replaces the project entry, and user-only labels are appended.

## API

The HTTP server (default `:7474`) exposes a small REST API. The endpoint most useful for scripting is ticket creation:

### `POST /api/boards/{id}/tickets`

Path parameter `{id}` accepts either the numeric board id or the board slug.

Body fields:

| Field       | Type    | Required | Notes                                                                                          |
| ----------- | ------- | -------- | ---------------------------------------------------------------------------------------------- |
| `title`     | string  | yes      |                                                                                                |
| `body`      | string  | no       | Markdown ticket description.                                                                   |
| `column_id` | integer | no       | Numeric column id. Wins over `column` if both set.                                             |
| `column`    | string  | no       | Column name (case-insensitive) or numeric string. Defaults to the leftmost column when omitted. |

Returns `201` with the created `Ticket` JSON, or `400` / `404` on validation failures. SSE subscribers on `/api/boards/{id}/events` receive a `ticket_created` event.

```bash
# Default column (Backlog), board by slug
curl -sX POST http://localhost:7474/api/boards/my-board/tickets \
     -H 'content-type: application/json' \
     -d '{"title":"Investigate flaky test"}' | jq

# Pick a column by name
curl -sX POST http://localhost:7474/api/boards/my-board/tickets \
     -H 'content-type: application/json' \
     -d '{"title":"Wire CI","column":"In Progress","body":"add a workflow"}' | jq

# Numeric ids (back-compat)
curl -sX POST http://localhost:7474/api/boards/1/tickets \
     -H 'content-type: application/json' \
     -d '{"title":"Compat","column_id":2}' | jq
```

The server has no authentication — bind only to `127.0.0.1` (the default container mapping does this).

## MCP

The same binary can run as a [Model Context Protocol](https://modelcontextprotocol.io) server over stdio, exposing kanban tools to AI agents. The MCP server is a thin client of the HTTP API above, so `kanban serve` must be running.

```bash
kanban mcp --server http://localhost:7474
# or via env
KANBAN_URL=http://localhost:7474 kanban mcp
```

Tools:

- `create_ticket` — args: `board` (id or slug), `title`, optional `body`, optional `column` (name or id, defaults to leftmost column).
- `list_boards` — returns `[{id, name, slug}]` for discovery.

### Claude Desktop

Add to `claude_desktop_config.json`:

```json
{
  "mcpServers": {
    "kanban": {
      "command": "/path/to/kanban",
      "args": ["mcp", "--server", "http://localhost:7474"]
    }
  }
}
```

### Claude Code

```bash
claude mcp add kanban -- /path/to/kanban mcp --server http://localhost:7474
```
