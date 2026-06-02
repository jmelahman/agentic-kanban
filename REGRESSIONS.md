# Recurring regression notes

Tripwires for code that has bitten us before. Each entry states the rule and
the trap, not the war story. They earn their keep when you hit them *before*
you knew you needed them — several fire from changes that don't look risky
(adding an SSE event, a routine handler that updates a couple of columns), so
skim the index in `CLAUDE.md` before touching the areas these name, and read
the full entry here before changing one.

When you fix a regression that fits an entry below, extend it. When you fix
something new and likely to recur, add a fresh entry here **and** a one-line
title to the index in `CLAUDE.md`. This file is deliberately kept out of the
published `docs/` site — it's internal engineering lore, not user docs.

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

### `ghostty-web` canvas doesn't fill its container

The bundled `FitAddon` reserves a hard-coded 15px scrollbar gutter (we draw
the scrollbar onto the canvas, so it's dead space) and ghostty sizes the
`<canvas>` to a whole-cell grid, so the canvas lands up to a cell short on the
right/bottom — a visible margin. `PtyTerminal.tsx` skips the addon, floors the
grid to whole cells against the full host box itself (`fitToHost`), then sets
the canvas inline `width/height` to `100%` to stretch the sub-cell remainder
away. Keep the fit in JS, not a CSS `!important` rule. ghostty rewrites the
inline canvas size only from `term.resize()` (which `fitToHost` owns) and font
changes (we never do at runtime), so re-applying the stretch inside `fitToHost`
is enough; if a package bump starts resetting it elsewhere, re-apply there too.

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

### Per-origin SSE streams starve the WebSocket pool

Browsers cap HTTP/1.1 connections at 6 per origin and route the initial
WebSocket upgrade request through the same pool. One `EventSource` per
board (Overview subscribes to *every* loaded board) saturates the pool
once you have ~6 boards open; the next PTY WebSocket handshake then sits
queued in `readyState: CONNECTING` indefinitely — the terminal panel
shows a cursor but never receives output until the user refreshes. All
SSE subscribers funnel through a singleton `BoardEventManager` in
`web/src/api/client.ts` that keeps one stream open against
`GET /api/events?boards=…`. Don't add new code paths that open per-board
EventSources; route them through `subscribeBoard` / `useBoardSubscription`.

### A deleted board can refetch its `/state` on a loop

There is no board-level SSE (no `board_created`/`board_deleted`), so the
boards list is only refreshed by *this* client's own mutations. A long-lived
tab that loaded while board N existed keeps board N mounted in Overview after
someone else deletes it — still observing `["board", N]` and still subscribed
via the multiplexed stream. Once we moved to one shared connection
(`26713ba`), every subscribed board's events arrive reliably (the old
per-board EventSources were starved by the 6-connection cap and silently
dropped these), so each event for the dead board re-invalidates and refetches
its 404ing `GET /api/boards/N/state` — amplified ×4 by react-query retries,
each a toast + `reportRuntimeError`. The `QueryClient` in `web/src/main.tsx`
breaks the loop: 4xx responses are never retried, and a 404 on a `["board",
…]` query refetches the boards list (dropping the dead board so its Overview
node unmounts) instead of toasting. If you ever add a board-level SSE event,
prefer evicting proactively over relying on this 404 fallback.
