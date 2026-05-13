import { api, waitForSession, isAttachable } from "./fixtures/api";
import { test, expect } from "./fixtures/seed";

// Full UI walk through a session: open the board, click a ticket, watch
// the auto-ensure + auto-start flow drive the status pill through
// stopped → starting → idle, terminal mounts, then stop returns it to
// stopped. Pins down `SessionView.tsx` auto-start + board-SSE consumer.
test("ticket click ensures, starts, and stops a session via UI", async ({
  page,
  seed,
}) => {
  await page.addInitScript((boardId) => {
    localStorage.setItem("kanban.activeBoardId", String(boardId));
  }, seed.board.id);

  const wsOpens: string[] = [];
  page.on("websocket", (ws) => wsOpens.push(ws.url()));

  await page.goto("/");

  const ticketCard = page.locator('[data-ticket-card="true"]').filter({
    hasText: seed.ticket.title,
  });
  await expect(ticketCard).toBeVisible();
  await ticketCard.click();

  // Wait for the session to flip into an attachable state. The UI
  // status pill mirrors the SSE-driven sessionStore.
  const session = await waitForSession(seed.board.id, await firstSessionId(seed.board.id, seed.ticket.id), isAttachable, {
    timeoutMs: 60_000,
  });
  expect(["idle", "working"]).toContain(session.status);

  // Status text on the ticket card matches the backend.
  await expect(ticketCard).toContainText(session.status);

  // The agent terminal mounts whenever a session is attachable.
  await expect(page.locator('[data-terminal="true"]').first()).toBeVisible();

  // A WebSocket to /ws/sessions/{id}/pty opened during the attach.
  expect(
    wsOpens.some((u) => u.includes(`/ws/sessions/${session.id}/pty`)),
  ).toBe(true);

  // Stop via the header action button (aria-label="stop").
  await page.getByRole("button", { name: "stop", exact: true }).click();

  await waitForSession(seed.board.id, session.id, (s) => s.status === "stopped", {
    timeoutMs: 30_000,
  });
  await expect(ticketCard).toContainText("stopped");
  await expect(page.locator('[data-terminal="true"]')).toHaveCount(0);
});

// Helper: poll boardState until a session exists for this ticket.
async function firstSessionId(boardId: number, ticketId: number): Promise<number> {
  const deadline = Date.now() + 20_000;
  while (Date.now() < deadline) {
    const state = await api.boardState(boardId);
    const found = state.sessions.find((s) => s.ticket_id === ticketId);
    if (found) return found.id;
    await new Promise((r) => setTimeout(r, 150));
  }
  throw new Error(`no session created for ticket ${ticketId} within 20s`);
}
