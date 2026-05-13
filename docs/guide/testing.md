# Testing

## Go

```bash
go test ./...
```

The `perfbench` build tag gates `internal/db/perfbench_test.go`. Run it
explicitly when changing schema/index code:

```bash
go test -tags=perfbench -run TestPerfReport -v ./internal/db/
```

## Frontend types

```bash
cd web
npm run typecheck
```

## End-to-end (Playwright)

The `web/tests/e2e/` suite drives the real frontend and backend through
Chromium. Sessions spawn real docker containers, so the broker, the
SSE bus, the terminal WebSocket, and the docker exec layer are all
exercised together.

### Prerequisites

- A running Docker daemon reachable from the shell.
- Network access on the first run so `lahmanja/kanban-devcontainer:latest`
  can be pulled. Override with `KANBAN_E2E_IMAGE=…` to use a different
  locally-available tag.
- Ports `7474` (backend) and `5174` (frontend) free, or already
  serving compatible processes — Playwright reuses an existing server
  outside CI.

### Run the suite

```bash
cd web
npm install
npm run test:e2e:install   # installs the Chromium browser playwright drives
npm run test:e2e
```

The first run pulls the session container image (a few minutes).
Subsequent runs take ~30–60 seconds.

Useful variants:

```bash
npx playwright test tests/e2e/session-lifecycle.spec.ts   # single spec
npm run test:e2e:ui                                       # interactive UI
```

### Running from the devcontainer

If you run the suite from inside this repo's devcontainer, the kanban
backend needs host-path translation so its container spawns can find
the bind mount sources on the host daemon. Set:

```bash
export KANBAN_HOST_WORKSPACE=/absolute/path/on/host/to/this/repo
export KANBAN_HOST_HOME=/absolute/path/on/host/to/your/home
```

The `globalSetup` already disables Claude config forwarding
(`--claude-config=false`) so the `/root/.claude` bind mount issue
described in `CLAUDE.md` doesn't apply. Mount paths the suite creates
live under `/tmp/kanban-e2e-*`, which both sides can stat.

### What's covered

| Spec | Validates |
| --- | --- |
| `session-lifecycle.spec.ts` | UI auto-ensure + auto-start flow, board SSE → status pill, terminal mount, stop. |
| `initial-load-scrollback.spec.ts` | First UI WS connect replays the ring buffer so prior output is visible immediately. |
| `ws-reconnect.spec.ts` | Server contract: every fresh shell WS receives `ESC c` + ring-buffer snapshot (`broker.go`). |
| `multiple-sessions.spec.ts` | Two concurrent sessions on one board don't cross-contaminate their WS streams. |

CI integration is not configured yet — the suite is local-only for now.
