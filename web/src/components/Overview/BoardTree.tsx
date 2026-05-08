import { useQueries, useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "@/api/client";
import { queryKeys } from "@/api/keys";
import { useBoardSubscription } from "@/hooks/useBoardSubscription";
import { fetchBoardStructure, useSession, useTicket } from "@/store";
import type { BoardStructure } from "@/store";
import { STATUS_BG, STATUS_BG_NONE } from "@/components/Ticket";
import {
  loadCollapsedBoards,
  writeCollapsedBoards,
} from "./storage";

export type OpenTicketFn = (boardId: number, ticketId: number) => void;

export function BoardTree({ onOpenTicket }: { onOpenTicket: OpenTicketFn }) {
  const boardsQ = useQuery({ queryKey: queryKeys.boards, queryFn: api.listBoards });
  const boards = boardsQ.data ?? [];
  const structures = useQueries({
    queries: boards.map((b) => ({
      queryKey: queryKeys.board(b.id),
      queryFn: () => fetchBoardStructure(b.id),
    })),
  });

  const [collapsed, setCollapsed] = useState<Set<number>>(loadCollapsedBoards);
  const toggle = (boardId: number) =>
    setCollapsed((prev) => {
      const next = new Set(prev);
      if (next.has(boardId)) next.delete(boardId);
      else next.add(boardId);
      writeCollapsedBoards(next);
      return next;
    });

  if (boardsQ.isLoading) {
    return <p className="p-3 text-sm text-fg-muted">Loading boards…</p>;
  }
  if (boards.length === 0) {
    return (
      <p className="p-3 text-sm text-fg-muted">
        No boards yet. Create one from the Board view.
      </p>
    );
  }

  return (
    <div className="flex h-full flex-col overflow-y-auto">
      <h2 className="border-b border-border px-3 py-2 text-xs font-semibold uppercase tracking-wide text-fg-muted">
        Boards
      </h2>
      {boards.map((b, i) => (
        <BoardNode
          key={b.id}
          boardId={b.id}
          boardName={b.name}
          structure={structures[i]?.data}
          collapsed={collapsed.has(b.id)}
          onToggle={() => toggle(b.id)}
          onOpenTicket={onOpenTicket}
        />
      ))}
    </div>
  );
}

function BoardNode({
  boardId,
  boardName,
  structure,
  collapsed,
  onToggle,
  onOpenTicket,
}: {
  boardId: number;
  boardName: string;
  structure: BoardStructure | undefined;
  collapsed: boolean;
  onToggle: () => void;
  onOpenTicket: OpenTicketFn;
}) {
  // Subscribe to live updates for *every* board in the tree so status dots
  // animate in real time. See useBoardSubscription doc for the connection
  // limit caveat.
  useBoardSubscription(boardId);

  const totalTickets = structure
    ? Object.values(structure.ticketIdsByColumn).reduce(
        (n, ids) => n + ids.length,
        0,
      )
    : 0;

  return (
    <div className="border-b border-border">
      <button
        type="button"
        onClick={onToggle}
        className="flex w-full items-center gap-2 px-3 py-2 text-left text-sm hover:bg-surface-2"
      >
        <span
          className={`inline-block w-3 text-xs text-fg-muted transition-transform ${collapsed ? "" : "rotate-90"}`}
        >
          ▶
        </span>
        <span className="font-medium">{boardName}</span>
        <span className="ml-auto text-xs text-fg-muted">{totalTickets}</span>
      </button>
      {!collapsed && structure && (
        <div className="pb-1">
          {structure.columns.map((c) => {
            const ids = structure.ticketIdsByColumn[c.id] ?? [];
            if (ids.length === 0) return null;
            return (
              <div key={c.id} className="px-3 py-1">
                <h3 className="mb-1 text-[10px] font-semibold uppercase tracking-wide text-fg-muted">
                  {c.name}
                </h3>
                <ul className="flex flex-col gap-0.5">
                  {ids.map((tid) => (
                    <TicketRow
                      key={tid}
                      ticketId={tid}
                      boardId={boardId}
                      sessionId={structure.sessionIdByTicket[tid] ?? null}
                      onOpenTicket={onOpenTicket}
                    />
                  ))}
                </ul>
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}

function TicketRow({
  ticketId,
  boardId,
  sessionId,
  onOpenTicket,
}: {
  ticketId: number;
  boardId: number;
  sessionId: number | null;
  onOpenTicket: OpenTicketFn;
}) {
  const ticket = useTicket(ticketId);
  const session = useSession(sessionId);
  if (!ticket) return null;
  const status = session?.status ?? "";
  const dotClass = status ? STATUS_BG[status] ?? STATUS_BG_NONE : STATUS_BG_NONE;
  return (
    <li>
      <button
        type="button"
        onClick={() => onOpenTicket(boardId, ticketId)}
        className="flex w-full items-center gap-2 rounded px-2 py-1 text-left text-sm hover:bg-surface-2"
        title={ticket.title}
      >
        <span className={`inline-block h-2 w-2 shrink-0 rounded-full ${dotClass}`} />
        <span className="truncate text-fg-muted">#{ticket.id}</span>
        <span className="truncate">{ticket.title}</span>
      </button>
    </li>
  );
}
