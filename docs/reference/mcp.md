# MCP

The same binary can run as a [Model Context Protocol](https://modelcontextprotocol.io) server over stdio, exposing kanban tools to AI agents. The MCP server is a thin client of the HTTP API, so `kanban serve` must be running.

```sh
kanban mcp --server http://localhost:7474
# or via env
KANBAN_URL=http://localhost:7474 kanban mcp
```

## Tools

### `create_ticket`

Create a ticket on a board.

| Argument  | Type    | Required | Notes                                                              |
|-----------|---------|----------|--------------------------------------------------------------------|
| `board`   | string  | yes      | Board id or slug.                                                  |
| `title`   | string  | yes      | Ticket title.                                                      |
| `body`    | string  | no       | Markdown ticket body.                                              |
| `column`  | string  | no       | Column name (case-insensitive) or id. Defaults to leftmost column. |

### `list_boards`

Returns `[{ id, name, slug }]` for discovery — useful when an agent needs to look up which board a ticket should land on.

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
Both wirings invoke `kanban mcp`, which is a stdio client. It still needs `kanban serve` to be reachable at the URL you pass — start the server first (or run it as a long-lived service / Docker container).
:::

## See also

- [REST API reference](./api) — what the MCP tools call under the hood.
- The MCP server source lives in [`internal/mcp/server.go`](https://github.com/jmelahman/agentic-kanban/blob/master/internal/mcp/server.go) if you want to add tools.
