# Quickstart

This walkthrough creates a board, adds a ticket, and starts an agent session — end to end in a few minutes.

::: tip Throwaway run
The whole walkthrough works against an in-memory database, so nothing is persisted to disk.

```sh
kanban serve --in-memory
```
:::

## 1. Open the UI

With `kanban serve` running, open <http://localhost:7474/>.

You'll land on the boards list. It's empty — there's nothing here yet.

![Empty boards list](/quickstart-01-empty.png)

## 2. Create a board

Click **New board**. Give it a name (e.g. `playground`) and point it at a local git repository on your machine. Kanban will read that repo's `.devcontainer/devcontainer.json` to figure out how to launch sessions for tickets.

![Create board dialog](/quickstart-02-create-board.png)

The board opens with the default columns: `Backlog`, `In Progress`, `In Review`, `Done`.

![Empty board](/quickstart-03-empty-board.png)

## 3. Create a ticket

Click **+** in the `Backlog` column. Title the ticket and write a markdown body describing what you want the agent to do.

![Create ticket](/quickstart-04-create-ticket.png)

When you save, the ticket appears as a card in `Backlog`.

## 4. Start the session

Open the ticket. The detail panel has four tabs — **agent**, **terminal**, **tasks**, **info**. Click **Start session**. Kanban will:

1. Create a git worktree off your base branch.
2. Spawn the worktree's devcontainer.
3. Launch the configured harness (Claude Code by default) inside it, attached to a PTY.

The harness's terminal streams into the **agent** tab; the **terminal** tab gives you a regular shell inside the same container.

## 5. Sync and merge

When the agent's done, the **Sync** menu offers to rebase or merge your base branch into the worktree (whichever you've allowed in `.kanban.toml`). The **Merge** menu sends the work back to the base branch via merge commit, squash, or rebase — again, configurable.

## What's next

- Edit `.kanban.toml` in your repo to lock in policy — see [Configuration](./configuration).
- Drive ticket creation from outside the UI — see the [REST API](/reference/api) and [CLI](/reference/cli) reference.
- Wire up Claude Desktop or Claude Code to create tickets via [MCP](/reference/mcp).
