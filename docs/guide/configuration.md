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
ai_commit_message  = false     # opt in to harness-generated messages for the auto-commit (default off — uses ticket title)

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

# Auto-start the configured harness inside a session container as soon as it
# spawns, using the ticket body as the prompt. Off by default — sessions boot
# to an idle container and the agent only runs once a user attaches the PTY.
# When on, kanban writes the ticket body to `.kanban/prompt.txt` inside the
# worktree and execs the harness's start template (e.g. `claude -p ...
# --dangerously-skip-permissions`) detached. Output is appended to
# `.kanban/agent.log` in the worktree. Tickets with an empty body are
# skipped. Requires a harness with a non-empty start template (claude has
# one; pi does not).
[agent]
auto_start = false

# Extra knobs layered onto the worktree's devcontainer.json at session spawn.
# `mounts` and `run_args` append to whatever the devcontainer.json declares;
# `container_env` merges with kanban values winning. `docker_socket` and
# `claude_config` only affect the built-in fallback devcontainer (both default
# to true) — hand-written devcontainer.json files manage their own mounts.
# `claude_config` can also be forced from the CLI via `--claude-config` or
# `$KANBAN_CLAUDE_CONFIG`, which take precedence over the toml value (useful
# when running kanban inside a devcontainer that already mounts `~/.claude`
# at a path the host's docker daemon can't see).
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

## Resuming Claude Code sessions across restarts

When a session container is restarted (or kanban itself is restarted and the container has to be recreated), the next `claude` launch automatically resumes the prior conversation for that ticket. There's nothing to configure — the mechanism is on by default whenever `~/.claude` is bind-mounted into the session container (the built-in devcontainer does this; see `claude_config = true` above).

How it works:

- On every `claude` startup, the bundled `.claude/settings.local.json` `SessionStart` hook PATCHes `/api/sessions/{id}/claude-session` with the conversation's UUID.
- Kanban stores the UUID on the session row.
- The next time the agent terminal is attached, kanban launches `claude --resume <uuid>` instead of a bare `claude`, so the transcript at `~/.claude/projects/<cwd-hash>/<uuid>.jsonl` is reopened.

To force a fresh conversation for a ticket, clear the stored UUID:

```sh
sqlite3 "$KANBAN_DATA_DIR/kanban.db" \
  "UPDATE sessions SET claude_session_id = NULL WHERE id = <session_id>"
```

The next attach will start a brand-new Claude Code session.

Caveats:

- Kanban only writes `.claude/settings.local.json` when no file is present. If one already exists — either hand-authored or shipped by an older kanban that didn't install the `SessionStart` hook — it's left untouched, no UUID is captured, and resume is silently off. To opt in, delete the file and re-ensure the session.
- Only the `claude` harness is wired up; other harnesses (`pi`) have no equivalent flag and launch fresh every time.

## Commit identity for merges

When a session is merged or squash-merged, kanban shells out to `git commit` inside its own container. If that container has no `user.name` / `user.email` configured, git aborts with `Author identity unknown` — your host's `~/.gitconfig` isn't visible inside the container.

Two ways to fix it, in order of preference:

1. **Set it on the board.** In the board settings UI, fill in *Commit identity → Author name / Author email*. Both fields must be set; leaving either blank falls back to whatever `git config` resolves inside the kanban container. Stored per-board (so different repos can use different identities), persisted as `git_author_name` / `git_author_email` on the board, and passed through as `-c user.name=… -c user.email=…` on every commit kanban creates.
2. **Mount your host gitconfig into the kanban container.** In `compose.yaml`, add `${HOME}/.gitconfig:/root/.gitconfig:ro` alongside the existing volumes. Useful if you want one identity across every board and don't mind the volume.

The board-level setting wins when both are present (it's an explicit `-c` flag on the git invocation).

## AI-generated commit messages

When kanban auto-commits a session's pending changes at merge time, it uses the ticket title as the message. Set `[merge].ai_commit_message = true` to instead invoke the session's harness (e.g. `claude --model haiku`) to generate a one-line message from the staged diff. The call is gated by a 90-second timeout and falls back to the ticket title on any error. Off by default because not every harness ships a working template, and the round-trip can be slow.

## Running kanban inside a container

When kanban itself runs inside a container (e.g. a devcontainer) but spawns
session containers via the host's docker daemon, every bind-mount source
kanban hands to dockerd needs to be a **host** path. Paths kanban sees as
`/workspace/...` or `/root/.claude` mean nothing to the host's docker.

Set these env vars on the kanban process to translate prefixes before they
reach dockerd:

| Env var | What it rewrites |
| --- | --- |
| `$KANBAN_HOST_WORKSPACE` | Host path of `/workspace` inside the kanban container. Covers board `repo_path`, `worktree_root`, and any `[devcontainer].mounts` whose source lives under `/workspace`. |
| `$KANBAN_HOST_HOME` | Host path of the kanban container's `$HOME`. Covers `~/.claude` / `~/.claude.json` forwarding. |

Example for a kanban devcontainer running as root with the project bind-mounted
from `/home/jamison/code/kanban`:

```sh
export KANBAN_HOST_WORKSPACE=/home/jamison/code/kanban
export KANBAN_HOST_HOME=/home/jamison
```

Both are unset (and the translation is a no-op) on the default host install.
Paths that don't start with either prefix pass through untouched.

For full end-to-end terminal access, the worktree root also needs to be
reachable on the host at the translated path — i.e. `KANBAN_HOST_WORKSPACE`
must point at a directory the host's docker daemon can stat.

## See also

- [CLI reference](/reference/cli) — every flag for `serve`, `mcp`, `board list`, `ticket create`.
- [REST API](/reference/api) — endpoints exposed by the running server.
