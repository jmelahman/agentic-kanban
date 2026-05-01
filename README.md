# kanban

A kanban board for managing AI agent sessions. Each ticket is bound to an agent session (Claude Code, pi/Ollama, …) running inside its own git worktree, executed in the target repository's existing devcontainer. The active harness is selected globally in the app's settings.

## Run

```bash
SOURCE=$HOME/code
docker run -d --name kanban \
  --restart unless-stopped \
  -p 127.0.0.1:7474:7474 \
  -p 13000-13099:13000-13099 \
  -v $XDG_RUNTIME_DIR/docker.sock:/var/run/docker.sock \
  -v $HOME/.claude:$HOME/.claude \
  # Agent config dir. Claude Code reads ~/.claude; for other harnesses
  # (e.g. pi/Ollama) substitute or add the relevant path.
  -v $SOURCE:$SOURCE \
  -v $HOME/.local/share/kanban:$HOME/.local/share/kanban \
  -e HOME=$HOME \
  -e XDG_RUNTIME_DIR=$XDG_RUNTIME_DIR \
  -e KANBAN_DATA_DIR=$HOME/.local/share/kanban \
  -e GH_TOKEN=$(gh auth token) \
  lahmanja/kanban:latest
```

Open `http://localhost:7474`.

## Build

```bash
cd kanban
docker build -t kanban:dev .
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
