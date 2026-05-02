import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, Ticket } from "@/api/client";
import { queryKeys } from "@/api/keys";
import { useToast } from "@/toast";
import { Button } from "./Button";
import { Drawer } from "./Modal";

export function ArchivedDrawer({
  open,
  boardId,
  onClose,
}: {
  open: boolean;
  boardId: number;
  onClose: () => void;
}) {
  const qc = useQueryClient();
  const { push } = useToast();
  const archivedQ = useQuery({
    queryKey: queryKeys.archived(boardId),
    queryFn: () => api.listArchivedTickets(boardId),
    enabled: open,
  });

  const deleteMut = useMutation({
    mutationFn: (id: number) => api.deleteTicket(id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.archived(boardId) });
      qc.invalidateQueries({ queryKey: queryKeys.board(boardId) });
      push("success", "Ticket and its resources deleted.");
    },
  });

  const unarchiveMut = useMutation({
    mutationFn: (id: number) => api.unarchiveTicket(id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.archived(boardId) });
      qc.invalidateQueries({ queryKey: queryKeys.board(boardId) });
      push("success", "Ticket unarchived.");
    },
  });

  return (
    <Drawer open={open} onClose={onClose} title="Archived tickets">
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
    </Drawer>
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
