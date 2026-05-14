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

# Build Cop polls GitHub Actions on a schedule and files tickets when a job's
# failure rate over a rolling window exceeds the threshold. Off by default.
# Each [[buildcop.boards]] entry produces one auto-managed board scoped to
# the given branch filter; columns are "Failing" / "Investigating" / "Fixed".
# A job auto-moves to "Fixed" once it has `green_streak_required` consecutive
# successful runs.
[buildcop]
enabled = false

[[buildcop.boards]]
name                  = "Build Cop: master"  # optional — defaults to "Build Cop: <branch>"
repo_path             = "/workspace"         # required: absolute path to a checkout with a GitHub origin
branch                = "master"             # "" or "*" matches every branch
failure_threshold     = 0.10                 # rate (0..1) above which a ticket is filed
min_runs              = 5                    # minimum runs in the window before evaluating
green_streak_required = 10                   # consecutive green runs to auto-move to Fixed
window_days           = 7                    # rolling window in days

# Extra knobs layered onto the worktree's devcontainer.json at session spawn.
# `mounts` and `run_args` append to whatever the devcontainer.json declares;
# `container_env` merges with kanban values winning. `docker_socket` and
# `claude_config` only affect the built-in fallback devcontainer —
# hand-written devcontainer.json files manage their own mounts.
# `docker_socket` defaults to false because forwarding the host daemon
# grants the session agent root-equivalent authority on the host; opt in
# only when a session legitimately needs to drive Docker. `claude_config`
# defaults to true and can also be forced from the CLI via
# `--claude-config` or `$KANBAN_CLAUDE_CONFIG`, which take precedence over
# the toml value (useful when running kanban inside a devcontainer that
# already mounts `~/.claude` at a path the host's docker daemon can't see).
[devcontainer]
mounts        = ["type=bind,source=/tmp/ssh-agent.sock,target=/tmp/ssh-agent.sock"]
run_args      = ["--cap-add=SYS_PTRACE"]
docker_socket = false          # built-in only: bind /var/run/docker.sock into the container
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

### Build Cop authentication

The poller calls the GitHub REST API the same way the PR-state poller does: it reads `$GH_TOKEN`, falls back to `$GITHUB_TOKEN`, then shells out to `gh auth token`. Without any of those, requests go out unauthenticated and hit the 60-requests-per-hour-per-IP limit almost immediately. For a typical board (one workflow, ~50 runs in 7 days) the poller needs a few hundred requests per hour, so use an authenticated token. `$GITHUB_API_URL` is honored for GitHub Enterprise Server hosts.

## Merge semantics

Most sections are object-merged: a key set in the user file wins; keys only set in the project file remain. Three sections behave specially:

- `[devcontainer].mounts` and `[devcontainer].run_args` are **appended** to whatever the worktree's `devcontainer.json` already declares. They aren't overrides.
- `[[task]]` entries merge by `label`: a user entry with the same `label` replaces the project entry, and user-only labels are appended.
- `[[buildcop.boards]]` is **replaced wholesale** when the user file sets any entries — board names can change and there's no stable identity to merge by, so the rule is "if the user declared boards, those are the boards."

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
| `$KANBAN_HOST_DOCKER_SOCK` | Host path of the docker socket. Needed when kanban's own `/var/run/docker.sock` is a bind of the host's rootless socket (e.g. `/var/run/user/1000/docker.sock`) — the in-container path is invalid as a bind source on the host. When unset, kanban probes `/var/run/docker.sock` then `$XDG_RUNTIME_DIR/docker.sock` directly. |

Example for a kanban devcontainer running as root with the project bind-mounted
from `/home/jamison/code/kanban`:

```sh
export KANBAN_HOST_WORKSPACE=/home/jamison/code/kanban
export KANBAN_HOST_HOME=/home/jamison
# Set on rootless-docker hosts where the socket isn't at /var/run/docker.sock:
export KANBAN_HOST_DOCKER_SOCK=/var/run/user/1000/docker.sock
```

Both are unset (and the translation is a no-op) on the default host install.
Paths that don't start with either prefix pass through untouched.

For full end-to-end terminal access, the worktree root also needs to be
reachable on the host at the translated path — i.e. `KANBAN_HOST_WORKSPACE`
must point at a directory the host's docker daemon can stat.

### Built-in session container user

When kanban spawns a session in the built-in devcontainer image, it picks
the in-container user by stat-ing the host's `~/.claude` and matching the
owner UID against the accounts the image ships: UID `0` → `root` (home
`/root`), UID `1000` → `dev` (home `/home/dev`). Other UIDs fall back to
`dev`.

The reason it matters: bind mounts preserve host UIDs verbatim, so if the
session ran as `dev` but `~/.claude` is root-owned (mode 0700 directory,
0600 credentials), `dev` couldn't read the credentials and Claude would
re-prompt `/login` every new session — and couldn't write the new
credentials back either, so the loop never broke.

Two env vars override the auto-pick when set. They resolve together, so
a session can't end up as `root` with `/home/dev` (or vice versa):

| Env var | Effect |
| --- | --- |
| `$DEVCONTAINER_REMOTE_USER` | In-container user (`dev` or `root`). If only this is set, the home is derived from a known-user table. |
| `$DEVCONTAINER_REMOTE_HOME` | In-container home prefix. If only this is set, the user still comes from the `~/.claude` auto-pick. |

For host UIDs the image doesn't ship an account for (e.g. macOS users
with UID 501), the auto-pick falls back to `dev`. Set the env vars
explicitly or chown the bind source to avoid the re-auth loop on those
hosts.

## See also

- [CLI reference](/reference/cli) — every flag for `serve`, `mcp`, `board list`, `ticket create`.
- [REST API](/reference/api) — endpoints exposed by the running server.
