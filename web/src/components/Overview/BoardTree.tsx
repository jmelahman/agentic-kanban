import { useMutation, useQueries, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "@/api/client";
import { queryKeys } from "@/api/keys";
import { useBoardSubscription } from "@/hooks/useBoardSubscription";
import { fetchBoardStructure, useSession, useTicket } from "@/store";
import type { BoardStructure } from "@/store";
import { STATUS_BG, STATUS_BG_NONE } from "@/components/Ticket";
import { Button } from "@/components/Button";
import {
  loadCollapsedBoards,
  writeCollapsedBoards,
} from "./storage";

export type OpenTicketFn = (boardId: number, ticketId: number) => void;

export function BoardTree({
  onOpenTicket,
  openTicketIds,
  onCollapseSidebar,
}: {
  onOpenTicket: OpenTicketFn;
  openTicketIds: ReadonlySet<number>;
  onCollapseSidebar?: () => void;
}) {
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

  const header = (
    <div className="flex items-center border-b border-border px-3 py-2">
      <h2 className="text-xs font-semibold uppercase tracking-wide text-fg-muted">
        Boards
      </h2>
      {onCollapseSidebar && (
        <button
          type="button"
          onClick={onCollapseSidebar}
          title="Hide boards sidebar"
          aria-label="Hide boards sidebar"
          aria-expanded={true}
          className="ml-auto rounded px-1 text-fg-muted hover:bg-surface-2 hover:text-fg"
        >
          <span aria-hidden>◀</span>
        </button>
      )}
    </div>
  );

  if (boardsQ.isLoading) {
    return (
      <div className="flex h-full flex-col">
        {header}
        <p className="p-3 text-sm text-fg-muted">Loading boards…</p>
      </div>
    );
  }
  if (boards.length === 0) {
    return (
      <div className="flex h-full flex-col">
        {header}
        <p className="p-3 text-sm text-fg-muted">
          No boards yet. Create one from the Board view.
        </p>
      </div>
    );
  }

  return (
    <div className="flex h-full flex-col overflow-y-auto">
      {header}
      {boards.map((b, i) => (
        <BoardNode
          key={b.id}
          boardId={b.id}
          boardName={b.name}
          structure={structures[i]?.data}
          collapsed={collapsed.has(b.id)}
          onToggle={() => toggle(b.id)}
          onOpenTicket={onOpenTicket}
          openTicketIds={openTicketIds}
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
  openTicketIds,
}: {
  boardId: number;
  boardName: string;
  structure: BoardStructure | undefined;
  collapsed: boolean;
  onToggle: () => void;
  onOpenTicket: OpenTicketFn;
  openTicketIds: ReadonlySet<number>;
}) {
  // Subscribe to live updates for *every* board in the tree so status dots
  // animate in real time. See useBoardSubscription doc for the connection
  // limit caveat.
  useBoardSubscription(boardId);

  const qc = useQueryClient();
  const [addingColumnId, setAddingColumnId] = useState<number | null>(null);
  const [title, setTitle] = useState("");

  const createMut = useMutation({
    mutationFn: (columnId: number) =>
      api.createTicket(boardId, { column_id: columnId, title }),
    onSuccess: () => {
      setTitle("");
      setAddingColumnId(null);
      qc.invalidateQueries({ queryKey: queryKeys.board(boardId) });
    },
  });

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
            const isAdding = addingColumnId === c.id;
            return (
              <div key={c.id} className="px-3 py-1">
                <div className="mb-1 flex items-center justify-between gap-2">
                  <h3 className="text-[10px] font-semibold uppercase tracking-wide text-fg-muted">
                    {c.name}
                  </h3>
                  <Button
                    variant="ghost"
                    size="sm"
                    type="button"
                    className="leading-none"
                    title={`Add ticket to ${c.name}`}
                    aria-label={`Add ticket to ${c.name}`}
                    onClick={() => {
                      setAddingColumnId(isAdding ? null : c.id);
                      setTitle("");
                    }}
                  >
                    +
                  </Button>
                </div>
                {isAdding && (
                  <form
                    onSubmit={(e) => {
                      e.preventDefault();
                      if (title.trim() && !createMut.isPending) createMut.mutate(c.id);
                    }}
                    className="mb-1 flex flex-col gap-1"
                  >
                    <input
                      autoFocus
                      className="rounded bg-surface-2 px-2 py-1 text-sm"
                      value={title}
                      onChange={(e) => setTitle(e.target.value)}
                      onKeyDown={(e) => {
                        if (e.key === "Escape") {
                          setAddingColumnId(null);
                          setTitle("");
                        }
                      }}
                      placeholder="ticket title"
                      disabled={createMut.isPending}
                    />
                    <div className="flex gap-2 text-xs">
                      <Button
                        variant="primary"
                        size="sm"
                        type="submit"
                        pending={createMut.isPending}
                        idleLabel="add"
                        pendingLabel="adding…"
                      />
                      <Button
                        variant="ghost"
                        size="sm"
                        type="button"
                        onClick={() => {
                          setAddingColumnId(null);
                          setTitle("");
                        }}
                        disabled={createMut.isPending}
                      >
                        cancel
                      </Button>
                    </div>
                  </form>
                )}
                {ids.length > 0 && (
                  <ul className="flex flex-col gap-0.5">
                    {ids.map((tid) => (
                      <TicketRow
                        key={tid}
                        ticketId={tid}
                        boardId={boardId}
                        sessionId={structure.sessionIdByTicket[tid] ?? null}
                        onOpenTicket={onOpenTicket}
                        active={openTicketIds.has(tid)}
                      />
                    ))}
                  </ul>
                )}
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
  active,
}: {
  ticketId: number;
  boardId: number;
  sessionId: number | null;
  onOpenTicket: OpenTicketFn;
  active: boolean;
}) {
  const ticket = useTicket(ticketId);
  const session = useSession(sessionId);
  if (!ticket) return null;
  const status = session?.status ?? "";
  const dotClass = status ? STATUS_BG[status] ?? STATUS_BG_NONE : STATUS_BG_NONE;
  const activeCls = active
    ? "bg-accent-500/15 ring-1 ring-inset ring-accent-500/40"
    : "hover:bg-surface-2";
  return (
    <li>
      <button
        type="button"
        onClick={() => onOpenTicket(boardId, ticketId)}
        className={`flex w-full items-center gap-2 rounded px-2 py-1 text-left text-sm ${activeCls}`}
        title={ticket.title}
        aria-current={active ? "true" : undefined}
      >
        <span className={`inline-block h-2 w-2 shrink-0 rounded-full ${dotClass}`} />
        <span className="truncate text-fg-muted">#{ticket.id}</span>
        <span className="truncate">{ticket.title}</span>
      </button>
    </li>
  );
}
