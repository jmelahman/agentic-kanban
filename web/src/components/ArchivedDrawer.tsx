import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect } from "react";
import { api, ApiError, Ticket } from "../api/client";
import { useToast } from "../toast";
import { Button } from "./Button";

export function ArchivedDrawer({ boardId, onClose }: { boardId: number; onClose: () => void }) {
  const qc = useQueryClient();
  const { push } = useToast();
  const archivedQ = useQuery({
    queryKey: ["archived", boardId],
    queryFn: () => api.listArchivedTickets(boardId),
  });

  const deleteMut = useMutation({
    mutationFn: (id: number) => api.deleteTicket(id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["archived", boardId] });
      qc.invalidateQueries({ queryKey: ["board", boardId] });
      push("success", "Ticket and its resources deleted.");
    },
    onError: (err) => {
      const msg = err instanceof ApiError ? err.message : err instanceof Error ? err.message : String(err);
      push("error", msg);
    },
  });

  const unarchiveMut = useMutation({
    mutationFn: (id: number) => api.unarchiveTicket(id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["archived", boardId] });
      qc.invalidateQueries({ queryKey: ["board", boardId] });
      push("success", "Ticket unarchived.");
    },
  });

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onClose]);

  return (
    <div className="fixed inset-0 z-40 flex" role="dialog" aria-modal="true">
      <div className="flex-1 bg-black/50" onClick={onClose} />
      <aside className="flex w-[480px] flex-col border-l border-zinc-800 bg-zinc-950">
        <header className="flex items-center justify-between border-b border-zinc-800 px-4 py-2">
          <h2 className="text-sm font-semibold">Archived tickets</h2>
          <Button variant="ghost" size="icon" onClick={onClose} aria-label="Close">
            ✕
          </Button>
        </header>
        <div className="flex-1 overflow-y-auto p-3">
          {archivedQ.isLoading && <p className="text-sm text-zinc-400">Loading…</p>}
          {archivedQ.data && archivedQ.data.length === 0 && (
            <p className="text-sm text-zinc-400">No archived tickets.</p>
          )}
          <ul className="flex flex-col gap-2">
            {(archivedQ.data ?? []).map((t) => (
              <ArchivedRow
                key={t.id}
                ticket={t}
                deletePending={deleteMut.isPending && deleteMut.variables === t.id}
                unarchivePending={unarchiveMut.isPending && unarchiveMut.variables === t.id}
                onUnarchive={() => unarchiveMut.mutate(t.id)}
                onDelete={() => {
                  if (
                    window.confirm(
                      `Permanently delete "${t.title}"?\n\nThis stops the container, removes the worktree, and deletes the branch.`,
                    )
                  ) {
                    deleteMut.mutate(t.id);
                  }
                }}
              />
            ))}
          </ul>
        </div>
      </aside>
    </div>
  );
}

function ArchivedRow({
  ticket,
  deletePending,
  unarchivePending,
  onDelete,
  onUnarchive,
}: {
  ticket: Ticket;
  deletePending: boolean;
  unarchivePending: boolean;
  onDelete: () => void;
  onUnarchive: () => void;
}) {
  const archivedAt = ticket.archived_at ? new Date(ticket.archived_at * 1000).toLocaleString() : "";
  const busy = deletePending || unarchivePending;
  return (
    <li className="rounded border border-zinc-800 bg-zinc-900 p-2 text-sm">
      <div className="flex items-start justify-between gap-2">
        <div className="min-w-0">
          <div className="font-medium">{ticket.title}</div>
          <div className="truncate font-mono text-xs text-zinc-500">{ticket.slug}</div>
          {archivedAt && <div className="text-xs text-zinc-500">archived {archivedAt}</div>}
        </div>
        <div className="flex shrink-0 gap-1">
          <Button
            variant="neutral"
            size="sm"
            onClick={onUnarchive}
            disabled={busy}
            pending={unarchivePending}
            idleLabel="unarchive"
            pendingLabel="unarchiving…"
          />
          <Button
            variant="danger"
            size="sm"
            onClick={onDelete}
            disabled={busy}
            pending={deletePending}
            idleLabel="delete"
            pendingLabel="deleting…"
          />
        </div>
      </div>
    </li>
  );
}
