# Configuration

Kanban reads two TOML files and merges them, with **user values overriding project values per key**:

- **Project**: `<repo>/.kanban.toml` — checked into the target repo, applies to every worktree of that repo.
- **User**: `$XDG_CONFIG_HOME/kanban/config.toml` (falling back to `~/.config/kanban/config.toml`) — your personal overrides across all repos.

Either file may be absent. Both accept the same schema.

## Schema

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

# Extra knobs layered onto the worktree's devcontainer.json at session spawn.
# `mounts` and `run_args` append to whatever the devcontainer.json declares;
# `container_env` merges with kanban values winning.
[devcontainer]
mounts   = ["type=bind,source=/tmp/ssh-agent.sock,target=/tmp/ssh-agent.sock"]
run_args = ["--cap-add=SYS_PTRACE"]

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

## See also

- [CLI reference](/reference/cli) — every flag for `serve`, `mcp`, `list-boards`, `create-ticket`.
- [REST API](/reference/api) — endpoints exposed by the running server.
