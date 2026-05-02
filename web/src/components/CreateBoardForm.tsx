import { useEffect, useState } from "react";
import { api, ApiError, Board } from "../api/client";
import { Button } from "./Button";

export function CreateBoardForm({ onCreated }: { onCreated: (b: Board) => void }) {
  const [open, setOpen] = useState(false);
  const [name, setName] = useState("");
  const [repo, setRepo] = useState("");
  const [base, setBase] = useState("main");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const close = () => {
    if (busy) return;
    setOpen(false);
    setError(null);
  };

  useEffect(() => {
    if (!open) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") close();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [open, busy]);

  return (
    <>
      <Button variant="neutral" size="lg" className="text-sm" onClick={() => setOpen(true)}>
        + new board
      </Button>
      {open && (
        <div className="fixed inset-0 z-40 flex items-center justify-center" role="dialog" aria-modal="true">
          <div className="absolute inset-0 bg-black/50" onClick={close} />
          <div className="relative w-[520px] max-w-[calc(100vw-2rem)] rounded border border-zinc-800 bg-zinc-950 shadow-lg">
            <header className="flex items-center justify-between border-b border-zinc-800 px-4 py-2">
              <h2 className="text-sm font-semibold">New board</h2>
              <Button
                variant="ghost"
                size="icon"
                onClick={close}
                disabled={busy}
                aria-label="Close"
              >
                ✕
              </Button>
            </header>
            <form
              className="flex flex-col gap-3 p-4 text-sm"
              onSubmit={async (e) => {
                e.preventDefault();
                setBusy(true);
                setError(null);
                try {
                  const board = await api.createBoard({ name, source_repo_path: repo, base_branch: base });
                  onCreated(board);
                  setOpen(false);
                  setName("");
                  setRepo("");
                  setBase("main");
                } catch (err) {
                  setError(err instanceof ApiError ? err.message : err instanceof Error ? err.message : String(err));
                } finally {
                  setBusy(false);
                }
              }}
            >
              <label className="flex flex-col gap-1">
                <span className="text-xs text-zinc-400">Name</span>
                <input
                  className="rounded bg-zinc-900 px-2 py-1"
                  placeholder="name"
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                  required
                  autoFocus
                />
              </label>
              <label className="flex flex-col gap-1">
                <span className="text-xs text-zinc-400">Repository path</span>
                <input
                  className="rounded bg-zinc-900 px-2 py-1"
                  placeholder="/host/path/to/repo"
                  value={repo}
                  onChange={(e) => setRepo(e.target.value)}
                  required
                />
              </label>
              <label className="flex flex-col gap-1">
                <span className="text-xs text-zinc-400">Base branch</span>
                <input
                  className="rounded bg-zinc-900 px-2 py-1"
                  placeholder="base branch"
                  value={base}
                  onChange={(e) => setBase(e.target.value)}
                />
              </label>
              {error && <div className="text-red-400 text-xs">{error}</div>}
              <div className="mt-2 flex items-center justify-end gap-2">
                <Button
                  type="button"
                  variant="ghost"
                  disabled={busy}
                  onClick={close}
                >
                  cancel
                </Button>
                <Button
                  type="submit"
                  variant="primary"
                  size="lg"
                  pending={busy}
                  idleLabel="create"
                  pendingLabel="creating…"
                />
              </div>
            </form>
          </div>
        </div>
      )}
    </>
  );
}
