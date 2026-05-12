import { useQueryClient } from "@tanstack/react-query";
import { useEffect } from "react";
import { PullProgress, Session, subscribeBoard, Ticket } from "@/api/client";
import { queryKeys } from "@/api/keys";
import {
  activeTicketStore,
  pullProgressStore,
  sessionStore,
  ticketStore,
} from "@/store";

export type StreamStatus = "open" | "error" | "closed";

// Subscribes to one board's SSE stream and routes events into the relevant
// caches. Safe to call from multiple components for the same board id —
// each call opens its own EventSource, so prefer mounting per *view*, not
// per *consumer*. Browsers cap HTTP/1.1 EventSource connections at 6 per
// origin; HTTP/2 lifts that. If board counts grow, plumb a multiplexed
// /api/events?boards=… endpoint and switch this hook to drive it.
export function useBoardSubscription(
  boardId: number | null,
  onStatus?: (s: StreamStatus) => void,
): void {
  const qc = useQueryClient();
  useEffect(() => {
    if (boardId == null) return;
    const key = queryKeys.board(boardId);
    return subscribeBoard(boardId, {
      onEvent: (type, data) => {
        applyBoardEvent(boardId, type, data, () =>
          qc.invalidateQueries({ queryKey: key }),
        );
        if (
          type === "ticket_archived" ||
          type === "ticket_unarchived" ||
          type === "ticket_deleted"
        ) {
          qc.invalidateQueries({ queryKey: queryKeys.archived(boardId) });
        }
      },
      onStatus: onStatus,
    });
  }, [boardId, qc, onStatus]);
}

// Routes an SSE event to the right cache. Per-id content updates flow
// straight to the entity stores (no cascade). Structural changes — anything
// that adds, removes, or reorders ticket-ids in a column — invalidate the
// boardStructure query so it refetches and rebuilds the index.
function applyBoardEvent(
  boardId: number,
  type: string,
  data: unknown,
  invalidateStructure: () => void,
): void {
  switch (type) {
    case "ticket_updated": {
      const t = data as Ticket | null;
      if (t) ticketStore.set(t.id, t);
      return;
    }
    case "ticket_created":
    case "ticket_moved":
    case "ticket_unarchived": {
      const t = data as Ticket | null;
      if (t) ticketStore.set(t.id, t);
      invalidateStructure();
      return;
    }
    case "ticket_archived":
    case "ticket_deleted": {
      const t = data as Ticket | null;
      if (t && activeTicketStore.get() === t.id && t.board_id === boardId) {
        activeTicketStore.set(null);
      }
      invalidateStructure();
      return;
    }
    case "session_updated": {
      const s = data as Session | null;
      if (!s) return;
      const prev = sessionStore.get(s.id);
      // While Restart's Stop phase is in flight, the DB still holds the OLD
      // container_id (manager.Stop clears it only after StopContainer +
      // RemoveContainer return — see manager.go:289-309). A Claude Code
      // status hook firing out of the dying container, or the GitHub PR
      // poller publishing a refetched row, will broadcast that stale shape
      // as session_updated{status: idle, container_id: OLD}. Without this
      // guard the optimistic "starting" set by restartMut.onMutate gets
      // clobbered, SessionTerminals re-mounts PtyTerminal, and the next
      // attach hits ExecAttachTTY against a container that's already gone —
      // surfacing as the "container X is not running" + [disconnected]
      // flash users see on Overview restart. The canonical session_updated
      // published from the restart endpoint's success path arrives with the
      // NEW container_id, which falls through this guard and unsticks the
      // state. Same pattern applies to a plain Stop's optimistic
      // "stopping".
      if (
        prev &&
        (prev.status === "starting" || prev.status === "stopping") &&
        s.status !== prev.status &&
        s.container_id != null &&
        s.container_id !== "" &&
        s.container_id === prev.container_id
      ) {
        return;
      }
      sessionStore.set(s.id, s);
      // Pull progress is only meaningful while the session is starting; clear
      // it on any other status so the bar doesn't linger after a stop/restart.
      if (s.status !== "starting") pullProgressStore.delete(s.id);
      // New session: ticket → session map needs rebuild.
      if (!prev) invalidateStructure();
      return;
    }
    case "session_pull_progress": {
      const p = data as PullProgress | null;
      if (!p) return;
      if (p.done) {
        pullProgressStore.delete(p.session_id);
      } else {
        pullProgressStore.set(p.session_id, p);
      }
      return;
    }
    case "ready":
    default:
      return;
  }
}
