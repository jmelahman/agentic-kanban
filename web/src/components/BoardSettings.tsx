import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useEffect, useState } from "react";
import { api, ApiError, Board } from "../api/client";
import { useToast } from "../toast";

export function BoardSettings({
  board,
  onClose,
  onDeleted,
}: {
  board: Board;
  onClose: () => void;
  onDeleted: () => void;
}) {
  const qc = useQueryClient();
  const { push } = useToast();
  const [name, setName] = useState(board.name);
  const [repo, setRepo] = useState(board.source_repo_path);
  const [base, setBase] = useState(board.base_branch);

  useEffect(() => {
    setName(board.name);
    setRepo(board.source_repo_path);
    setBase(board.base_branch);
  }, [board.id, board.name, board.source_repo_path, board.base_branch]);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onClose]);

  const updateMut = useMutation({
    mutationFn: () =>
      api.updateBoard(board.id, {
        name: name.trim(),
        source_repo_path: repo.trim(),
        base_branch: base.trim(),
      }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["boards"] });
      qc.invalidateQueries({ queryKey: ["board", board.id] });
      push("success", "Board updated.");
      onClose();
    },
    onError: (err) => {
      const msg = err instanceof ApiError ? err.message : err instanceof Error ? err.message : String(err);
      push("error", msg);
    },
  });

  const deleteMut = useMutation({
    mutationFn: () => api.deleteBoard(board.id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["boards"] });
      push("success", `Deleted board "${board.name}".`);
      onDeleted();
    },
    onError: (err) => {
      const msg = err instanceof ApiError ? err.message : err instanceof Error ? err.message : String(err);
      push("error", msg);
    },
  });

  const dirty = name.trim() !== board.name || repo.trim() !== board.source_repo_path || base.trim() !== board.base_branch;
  const valid = name.trim() !== "" && repo.trim() !== "" && base.trim() !== "";
  const busy = updateMut.isPending || deleteMut.isPending;

  return (
    <div className="fixed inset-0 z-40 flex items-center justify-center" role="dialog" aria-modal="true">
      <div className="absolute inset-0 bg-black/50" onClick={busy ? undefined : onClose} />
      <div className="relative w-[520px] max-w-[calc(100vw-2rem)] rounded border border-zinc-800 bg-zinc-950 shadow-lg">
        <header className="flex items-center justify-between border-b border-zinc-800 px-4 py-2">
          <h2 className="text-sm font-semibold">Board settings</h2>
          <button
            className="text-zinc-400 hover:text-zinc-100 disabled:opacity-50"
            onClick={onClose}
            disabled={busy}
            aria-label="Close"
          >
            ✕
          </button>
        </header>
        <form
          className="flex flex-col gap-3 p-4 text-sm"
          onSubmit={(e) => {
            e.preventDefault();
            if (!dirty || !valid) return;
            updateMut.mutate();
          }}
        >
          <label className="flex flex-col gap-1">
            <span className="text-xs text-zinc-400">Name</span>
            <input
              className="rounded bg-zinc-900 px-2 py-1"
              value={name}
              onChange={(e) => setName(e.target.value)}
              required
            />
          </label>
          <label className="flex flex-col gap-1">
            <span className="text-xs text-zinc-400">Source repo path</span>
            <input
              className="rounded bg-zinc-900 px-2 py-1 font-mono"
              value={repo}
              onChange={(e) => setRepo(e.target.value)}
              required
            />
          </label>
          <label className="flex flex-col gap-1">
            <span className="text-xs text-zinc-400">Base branch</span>
            <input
              className="rounded bg-zinc-900 px-2 py-1"
              value={base}
              onChange={(e) => setBase(e.target.value)}
              required
            />
          </label>
          <div className="flex flex-col gap-1 text-xs text-zinc-500">
            <span>slug: <span className="font-mono">{board.slug}</span></span>
            <span>worktree root: <span className="font-mono">{board.worktree_root}</span></span>
          </div>
          <div className="mt-2 flex items-center justify-end gap-2">
            <button
              type="button"
              className="text-zinc-400 hover:text-zinc-100 disabled:opacity-50"
              onClick={onClose}
              disabled={busy}
            >
              cancel
            </button>
            <button
              type="submit"
              disabled={!dirty || !valid || busy}
              className="rounded bg-red-700 px-3 py-1 text-white disabled:opacity-50"
            >
              {updateMut.isPending ? "saving…" : "save"}
            </button>
          </div>
        </form>
        <div className="border-t border-zinc-800 p-4">
          <h3 className="text-xs font-semibold uppercase tracking-wide text-red-400">Danger zone</h3>
          <p className="mt-1 text-xs text-zinc-400">
            Deletes this board, all its tickets, and stops/destroys every running session
            (containers, worktrees, and branches).
          </p>
          <button
            type="button"
            className="mt-2 rounded bg-red-900/60 px-3 py-1 text-xs text-red-100 hover:bg-red-800 disabled:opacity-50"
            disabled={busy}
            onClick={() => {
              if (
                window.confirm(
                  `Permanently delete board "${board.name}"?\n\nThis stops all containers, removes worktrees, deletes branches, and removes every ticket.`,
                )
              ) {
                deleteMut.mutate();
              }
            }}
          >
            {deleteMut.isPending ? "deleting…" : "delete board"}
          </button>
        </div>
      </div>
    </div>
  );
}
