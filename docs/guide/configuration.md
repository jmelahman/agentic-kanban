# Configuration

Kanban reads two TOML files and merges them, with **user values overriding project values per key**:

- **Project**: `<repo>/.kanban.toml` — checked into the target repo, applies to every worktree of that repo.
- **User**: `$XDG_CONFIG_HOME/kanban/config.toml` (falling back to `~/.config/kanban/config.toml`) — your personal overrides across all repos.

Either file may be absent. Both accept the same schema.

## Schema

```toml
[harness]
id = "claude-code"            # default harness for new sessions

[worktrees]
root = "/path/to/worktrees"   # parent dir for new worktrees (overrides --worktrees-dir)

[branches]
prefix = "kanban"              # branch name = "<prefix>/<ticket-slug>"

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

# Toggles the in-process error-to-ticket reporter. Off by default — typically
# only useful to developers maintaining kanban itself.
[errors]
enabled    = false
board_name = "kanban-errors"

# Extra knobs layered onto the worktree's devcontainer.json at session spawn.
# `mounts` and `run_args` append to whatever the devcontainer.json declares;
# `container_env` merges with kanban values winning. `docker_socket` and
# `claude_config` only affect the built-in fallback devcontainer (both default
# to true) — hand-written devcontainer.json files manage their own mounts.
[devcontainer]
mounts        = ["type=bind,source=/tmp/ssh-agent.sock,target=/tmp/ssh-agent.sock"]
run_args      = ["--cap-add=SYS_PTRACE"]
docker_socket = true           # built-in only: bind /var/run/docker.sock into the container
claude_config = true           # built-in only: bind ~/.claude into the container

[devcontainer.container_env]
SSH_AUTH_SOCK = "/tmp/ssh-agent.sock"

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

## Merge semantics

Most sections are object-merged: a key set in the user file wins; keys only set in the project file remain. Two sections behave specially:

- `[devcontainer].mounts` and `[devcontainer].run_args` are **appended** to whatever the worktree's `devcontainer.json` already declares. They aren't overrides.
- `[[task]]` entries merge by `label`: a user entry with the same `label` replaces the project entry, and user-only labels are appended.

## Overriding the user-config path

```sh
kanban serve --config /path/to/config.toml
# or
KANBAN_CONFIG=/path/to/config.toml kanban serve
```

The `--config` flag wins over `$KANBAN_CONFIG`.

## Per-task ports

Kanban understands `.vscode/tasks.json` in the target repo. When a task whose `label` matches a `[[task]]` entry starts inside a session, kanban allocates the next free host port in the range (default `13000–13099`) and runs a TCP proxy from that host port to the declared `container_port`. The host port is shown on the ticket so you can open the WIP service in your browser.

Adjust the proxy range with `--port-range-start` and `--port-range-end` on `kanban serve`.

## Commit identity for merges

When a session is merged or squash-merged, kanban shells out to `git commit` inside its own container. If that container has no `user.name` / `user.email` configured, git aborts with `Author identity unknown` — your host's `~/.gitconfig` isn't visible inside the container.

Two ways to fix it, in order of preference:

1. **Set it on the board.** In the board settings UI, fill in *Commit identity → Author name / Author email*. Both fields must be set; leaving either blank falls back to whatever `git config` resolves inside the kanban container. Stored per-board (so different repos can use different identities), persisted as `git_author_name` / `git_author_email` on the board, and passed through as `-c user.name=… -c user.email=…` on every commit kanban creates.
2. **Mount your host gitconfig into the kanban container.** In `compose.yaml`, add `${HOME}/.gitconfig:/root/.gitconfig:ro` alongside the existing volumes. Useful if you want one identity across every board and don't mind the volume.

The board-level setting wins when both are present (it's an explicit `-c` flag on the git invocation).

## See also

- [CLI reference](/reference/cli) — every flag for `serve`, `mcp`, `list-boards`, `create-ticket`.
- [REST API](/reference/api) — endpoints exposed by the running server.
