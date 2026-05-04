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

## Tests / typecheck

- Go: `go test ./...`
- Frontend types: `cd web && npm run typecheck`

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
- `.kanban.toml` — maps the task labels above to container ports so kanban
  proxies them to host ports `13000–13099` when run as a session.
- `.devcontainer/` — image, firewall (allowlists `storage.googleapis.com`,
  `registry.npmjs.org`, GitHub, `proxy.golang.org`, anthropic, etc.).

## Network notes

Outbound HTTP from this container is allowlisted. If a fetch fails with
`No route to host`, the destination is firewalled — don't try to work around
it; pick an allowed mirror or tell the user.
