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

## Tests / typecheck / lint

- Go: `go test ./...`
- Frontend types: `cd web && npm run typecheck`
- Frontend lint/format (Biome): `cd web && npm run check` (`check:fix` to
  auto-apply safe fixes). The `prek` `biome` hook runs the same check on
  staged `web/**/*.{ts,tsx,js,jsx,json}` files.
- Playwright E2E: `cd web && npm run test:e2e`. Read
  `web/tests/e2e/README.md` before adding or modifying a spec.
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

## Metrics

`internal/metrics` exposes a shared Prometheus registry served at
`GET /metrics`. Four signals are wired:

- HTTP server (middleware in `cmd/server/main.go`, labelled by ServeMux
  pattern).
- GitHub REST API (RoundTripper installed on both `internal/github` and
  `internal/buildcop` clients; records counters, latency, and
  `X-RateLimit-*` headers).
- SQLite queries (driver wrapper registered as `sqlite-instrumented`; used
  by `internal/db.Open`).
- Go runtime / process collectors.

When adding a new HTTP client that calls GitHub, wrap its `Transport` with
`metrics.WrapGitHubTransport(nil)` — otherwise it won't show up in the
rate-limit gauges.

`compose.yaml` bundles a Prometheus service; the UI is exposed under
`/prometheus/` via a reverse-proxy on the kanban backend (driven by
`$KANBAN_PROMETHEUS_URL`). See `docs/guide/observability.md`.

From inside this devcontainer (attached to `kanban-net`), Prometheus is
reachable directly at `http://prometheus:9090/prometheus/` — the server
is configured with `--web.external-url=/prometheus`, so the API lives
under `/prometheus/api/v1/...` (a bare `/api/v1/...` returns 404). Useful
probes:

```bash
# Build / version
curl -sf http://prometheus:9090/prometheus/api/v1/status/buildinfo

# Active scrape targets (expect kanban:7474 -> up)
curl -sf 'http://prometheus:9090/prometheus/api/v1/targets?state=active'

# Instant query
curl -sf --data-urlencode 'query=up' \
  http://prometheus:9090/prometheus/api/v1/query
```

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

### `ghostty-web` terminal dispose poisons the WASM heap

`Terminal.dispose()` calls `ghostty_terminal_free`, which corrupts the
shared WASM linear memory whenever the terminal previously wrote a
multi-codepoint grapheme cluster (flag emoji, skin tone, ZWJ family,
keycap). Because `init()` keeps a single page-wide Ghostty instance, the
next terminal's first `write()` traps with "Out of bounds memory access"
— see upstream issue coder/ghostty-web#141. `PtyTerminal.tsx` works
around this by setting the private `wasmTerm` field to `undefined`
before calling `dispose()`, which skips the buggy `free()` while keeping
the rest of cleanup (DOM removal, document listeners, observers). Drop
the workaround when coder/ghostty-web#142 lands and we bump the package.

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
