# MCP

The same binary can run as a [Model Context Protocol](https://modelcontextprotocol.io)
server over stdio, exposing kanban tools to AI agents. The MCP server is a
thin client of the HTTP API, so `kanban serve` must be running.

```sh
kanban mcp --server http://localhost:7474
# or via env
KANBAN_URL=http://localhost:7474 kanban mcp
```

## Tools

Tool names use `verb_noun` snake_case. Read tools (`list_*`, `get_*`,
`board_state`) return JSON; mutating tools that 204 from the REST layer
return the literal string `"ok"`.

### Boards

#### `list_boards`

Returns `[{ id, name, slug }]` for all boards.

#### `create_board`

Create a new board. The server seeds the default columns
(Backlog / In Progress / Review / Done).

| Argument         | Type   | Required | Notes                                                              |
|------------------|--------|----------|--------------------------------------------------------------------|
| `name`           | string | yes      | Board name.                                                        |
| `repo_path`      | string | one of   | Host git repo path. Required if `mount_path` is empty.             |
| `mount_path`     | string | one of   | Mount path inside session containers.                              |
| `worktree_root`  | string | no       | Override the parent directory for new session worktrees.           |
| `base_branch`    | string | no       | Branch session worktrees fork from. Detected from `repo_path` (`origin/HEAD`, then current branch) when omitted; falls back to `main`. |
| `branch_prefix`  | string | no       | Prefix for session branch names.                                   |
| `git_author_name`  | string | no     | Author name used for merge/squash commits the server creates.     |
| `git_author_email` | string | no     | Author email used for merge/squash commits.                       |

#### `get_board`

| Argument | Type   | Required | Notes                                |
|----------|--------|----------|--------------------------------------|
| `board`  | string | yes      | Numeric board id or slug.            |

#### `update_board`

Patch fields on a board. Only fields you supply are updated.

| Argument         | Type   | Required | Notes                              |
|------------------|--------|----------|------------------------------------|
| `board`          | string | yes      | Board id or slug.                  |
| `name`           | string | no       | New name.                          |
| `repo_path`      | string | no       | New repo path.                     |
| `mount_path`     | string | no       | New mount path.                    |
| `worktree_root`  | string | no       | New worktree root.                 |
| `base_branch`    | string | no       | New base branch.                   |
| `branch_prefix`  | string | no       | New branch prefix.                 |
| `git_author_name`  | string | no     | New commit author name.            |
| `git_author_email` | string | no     | New commit author email.           |

#### `delete_board`

Destroys every associated session, then deletes the board.

| Argument | Type   | Required | Notes                                |
|----------|--------|----------|--------------------------------------|
| `board`  | string | yes      | Board id or slug.                    |

#### `board_state`

Single-shot snapshot: `{board, columns, tickets, sessions, merge_config,
sync_config}`.

| Argument | Type   | Required | Notes                                |
|----------|--------|----------|--------------------------------------|
| `board`  | string | yes      | Board id or slug.                    |

#### `list_archived` / `delete_archived`

List or permanently delete every archived ticket on a board. `delete_archived`
is destructive.

| Argument | Type   | Required | Notes                                |
|----------|--------|----------|--------------------------------------|
| `board`  | string | yes      | Board id or slug.                    |

### Tickets

#### `create_ticket`

Create a ticket on a board.

| Argument  | Type    | Required | Notes                                                              |
|-----------|---------|----------|--------------------------------------------------------------------|
| `board`   | string  | yes      | Board id or slug.                                                  |
| `title`   | string  | yes      | Ticket title.                                                      |
| `body`    | string  | no       | Markdown ticket body.                                              |
| `column`  | string  | no       | Column name (case-insensitive) or id. Defaults to leftmost column. |

#### `update_ticket`

Patch ticket fields.

| Argument | Type    | Required | Notes        |
|----------|---------|----------|--------------|
| `ticket` | integer | yes      | Ticket id.   |
| `title`  | string  | no       | New title.   |
| `body`   | string  | no       | New body.    |

#### `move_ticket`

| Argument    | Type    | Required | Notes                                |
|-------------|---------|----------|--------------------------------------|
| `ticket`    | integer | yes      | Ticket id.                           |
| `column_id` | integer | yes      | Target column id.                    |
| `position`  | integer | no       | 0-indexed position. Default `0`.     |

#### `archive_ticket` / `unarchive_ticket` / `delete_ticket` / `done_ticket`

All take a single `ticket` (integer) argument. `delete_ticket` requires
the ticket to already be archived; `done_ticket` moves the ticket to the
board's rightmost column and stops its session.

#### `sync_ticket`

| Argument   | Type    | Required | Notes                                  |
|------------|---------|----------|----------------------------------------|
| `ticket`   | integer | yes      | Ticket id.                             |
| `strategy` | string  | no       | `rebase` or `merge`. Default `rebase`. |

#### `merge_ticket`

| Argument   | Type    | Required | Notes                                                |
|------------|---------|----------|------------------------------------------------------|
| `ticket`   | integer | yes      | Ticket id.                                           |
| `strategy` | string  | yes      | `merge-commit`, `squash`, or `rebase`.               |

### Columns

#### `archive_column_tickets`

Archives every non-archived ticket in the column. Stops their sessions.

| Argument    | Type    | Required | Notes                |
|-------------|---------|----------|----------------------|
| `column_id` | integer | yes      | Column id (numeric). |

### Sessions

#### `ensure_session`

Ensures a session exists for the given ticket; creates one if absent.

| Argument | Type    | Required | Notes        |
|----------|---------|----------|--------------|
| `ticket` | integer | yes      | Ticket id.   |

#### `start_session` / `stop_session` / `restart_session`

| Argument  | Type    | Required | Notes        |
|-----------|---------|----------|--------------|
| `session` | integer | yes      | Session id.  |

## Wiring

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

Restart Claude Desktop. The kanban tools should appear in the tools picker.

### Claude Code

```sh
claude mcp add kanban -- /path/to/kanban mcp --server http://localhost:7474
```

Run `claude mcp list` to confirm the server registered.

::: tip Server must be running
Both wirings invoke `kanban mcp`, which is a stdio client. It still needs
`kanban serve` to be reachable at the URL you pass — start the server
first (or run it as a long-lived service / Docker container).
:::

## See also

- [REST API reference](./api) — what the MCP tools call under the hood.
- The MCP server source lives in [`internal/mcp/server.go`](https://github.com/jmelahman/agentic-kanban/blob/master/internal/mcp/server.go) if you want to add tools.
