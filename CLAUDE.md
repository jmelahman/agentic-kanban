# Agentic Kanban — Claude Notes

A Go backend (`/workspace`) plus a React/Vite frontend (`/workspace/web`).
Source of truth for run commands is `.vscode/tasks.json`; this file translates
them for use inside this devcontainer.

## Running the app

Both processes are long-running. Start them with `run_in_background: true`,
then poll until their ports answer before driving the UI.

**Backend** (`:7474`):

```bash
wgo run . serve
```

**Frontend** (`:5173`, talks to backend in the same container):

```bash
cd web && npm install && npm run dev -- --host 0.0.0.0
```

**Frontend against a host backend** (`:5174`, points at a backend running on
the host at `localhost:7474`):

```bash
cd web && KANBAN_BACKEND=localhost:7474 npm install && npm run dev -- --host 0.0.0.0 --port 5174
```

Wait for both to be reachable before navigating:

```bash
until curl -sf -m 1 http://localhost:7474/api/boards >/dev/null \
   && curl -sf -m 1 http://localhost:5173/ >/dev/null; do sleep 2; done
```

## Reproducing fresh-install issues

Boot a clean instance with no on-disk DB:

```bash
wgo run . serve --in-memory
```

The server logs `WARNING: --in-memory set` at startup and uses an ephemeral
SQLite database (shared cache, single connection) for the lifetime of the
process. Each launch starts from zero, and shutting the process down
discards everything — including any boards or tickets the UI created.
Frontend (`:5173`) and Playwright MCP (`browser_navigate
http://localhost:5173/`) work as usual.

## Spawning session containers from this devcontainer

The host runs rootless Docker as a non-root user, but this devcontainer
runs as root and stores `~/.claude*` under `/root/.claude*`. When kanban
forwards Claude config into a session container it tells dockerd to bind
`source=/root/.claude` — a path the rootless dockerd on the host can't
stat, so session start fails with `permission denied`. The same kind of
path-aliasing issue affects the worktrees dir and any board whose
`repo_path` doesn't resolve identically inside and outside this container.

For tests where you only need an unprivileged shell session (no agent),
disable claude config forwarding for that one server invocation:

```bash
wgo run . serve --in-memory --claude-config=false
```

The flag/env (`$KANBAN_CLAUDE_CONFIG`) overrides
`.kanban.toml [devcontainer].claude_config` for that process only — the
project toml stays as-is so other developers/sessions still get Claude
forwarded normally.

End-to-end terminal access from inside this devcontainer also requires
host-path translation: paths kanban sees as `/workspace/...` and
`/root/.claude` need to be rewritten to the corresponding host paths
before being handed to dockerd. Set these on the kanban process:

```sh
export KANBAN_HOST_WORKSPACE=/path/on/host/to/this/repo   # rewrites /workspace prefix
export KANBAN_HOST_HOME=/path/on/host/to/your/home        # rewrites $HOME prefix
export KANBAN_HOST_DOCKER_SOCK=/var/run/user/1000/docker.sock  # rootless docker socket
```

All three default to unset (no translation) so the host install is unaffected.
`KANBAN_HOST_DOCKER_SOCK` is only needed on rootless-docker hosts where
the devcontainer's `/var/run/docker.sock` is a bind of e.g.
`/var/run/user/$UID/docker.sock` — handing the in-container path to the
host's dockerd fails with `bind source path does not exist`.
See `docs/guide/configuration.md` § "Running kanban inside a container"
for the full table.

## Tests / typecheck / lint

- Go: `go test ./...`
- Frontend types: `cd web && npm run typecheck`
- Pre-commit hooks: `prek run --all-files` (run before committing).

## Perf benchmark

`internal/db/perfbench_test.go` is a build-tagged comparison of hot DB
queries with vs. without the `idx_*` indexes from `schema.sql`. It seeds
an in-memory SQLite at n=100/1000/5000 tickets, times each query, drops
the indexes, re-times, and prints a speedup table.

```bash
go test -tags=perfbench -run TestPerfReport -v ./internal/db/
```

The `perfbench` build tag keeps it out of the default `go test ./...`.
Use it to validate future schema/query changes — a regression on the
`MaxPosition` or `ListArchived` rows is the canary (those are 30–95×
faster with indexes at n≥1000; everything else is a wash on small tables).

## Driving the UI with Playwright MCP

`.mcp.json` registers `@playwright/mcp --headless --isolated`. Use the
`mcp__playwright__browser_*` tools (e.g. `browser_navigate
http://localhost:5173/`, then `browser_snapshot`) — never spawn `npx
playwright` ad-hoc.

`@playwright/mcp` defaults to the `chrome` channel which looks at
`/opt/google/chrome/chrome`; the devcontainer Dockerfile symlinks that to the
bundled Chromium under `$PLAYWRIGHT_BROWSERS_PATH=/ms-playwright`.

## Layout

- `main.go`, `cmd/`, `internal/` — Go server, MCP, CLI subcommands.
- `web/` — Vite + React 19 + Tailwind 4 frontend.
- `docs/` — VitePress site (`guide/`, `reference/api.md`, `reference/cli.md`,
  `reference/mcp.md`).
- `.kanban.toml` — maps the task labels above to container ports so kanban
  proxies them to host ports `13000–13099` when run as a session.
- `.devcontainer/` — image, firewall (allowlists `storage.googleapis.com`,
  `registry.npmjs.org`, GitHub, `proxy.golang.org`, anthropic, etc.).

## Recurring regression notes

Short entries on bugs that have bitten us before. When you fix a regression
that fits a pattern below, extend the entry; when you fix something new and
likely to recur, add a fresh one. Keep each entry short — state the rule,
not the war story.

### `sessions` row has multiple writers

Two independent paths write to the `sessions` row: the session manager
(lifecycle columns — `status`, `container_id`, `started_at`, `stopped_at`)
and the GitHub poller (`pr_state`, `pr_number`, `pr_url`, `pr_title`).
Anything that loads the row, mutates a few fields, then writes the whole
row back will silently clobber whatever the other writer just committed.
Rules:

- Use column-scoped updates (`UpdateSessionLifecycle`, `UpdateSessionPR`).
  Don't `UpsertSession` from a path that doesn't own every column.
- Before publishing `session_updated` over SSE, refetch from the DB so the
  wire payload reflects what's persisted. HTTP handlers funnel through
  `publishSessionUpdated(ctx, sessionID)` which refetches; the poller
  refetches in `applyTransition`'s defer.

## Documentation upkeep

User-facing changes need a docs update in the same PR. Match the change to
the page:

- New/changed CLI flags or subcommands → `docs/reference/cli.md`.
- New/changed HTTP endpoints or request/response shapes → `docs/reference/api.md`.
- New/changed MCP tools → `docs/reference/mcp.md`.
- Config keys (`.kanban.toml`, env vars, `--in-memory`, etc.) → `docs/guide/configuration.md`.
- Install/setup steps → `docs/guide/install.md` or `docs/guide/quickstart.md`.

If a feature doesn't fit an existing page, add one under `docs/guide/` and
link it from `docs/.vitepress/config.*`. Skip docs only for purely internal
refactors with no observable behavior change.

## Network notes

Outbound HTTP from this container is allowlisted. If a fetch fails with
`No route to host`, the destination is firewalled — don't try to work around
it; pick an allowed mirror or tell the user.
