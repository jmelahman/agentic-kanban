import { useState } from "react";
import { api, ApiError, Board } from "../api/client";
import { Button } from "./Button";

export function CreateBoardForm({ onCreated }: { onCreated: (b: Board) => void }) {
  const [open, setOpen] = useState(false);
  const [name, setName] = useState("");
  const [repo, setRepo] = useState("");
  const [base, setBase] = useState("main");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  if (!open) {
    return (
      <Button variant="neutral" size="lg" className="text-sm" onClick={() => setOpen(true)}>
        + new board
      </Button>
    );
  }

  return (
    <form
      className="flex flex-col gap-1 text-sm"
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
        } catch (err) {
          setError(err instanceof ApiError ? err.message : err instanceof Error ? err.message : String(err));
        } finally {
          setBusy(false);
        }
      }}
    >
      <div className="flex items-center gap-2">
        <input className="rounded bg-zinc-900 px-2 py-1" placeholder="name" value={name} onChange={(e) => setName(e.target.value)} required />
        <input className="rounded bg-zinc-900 px-2 py-1 w-72" placeholder="/host/path/to/repo" value={repo} onChange={(e) => setRepo(e.target.value)} required />
        <input className="rounded bg-zinc-900 px-2 py-1 w-28" placeholder="base branch" value={base} onChange={(e) => setBase(e.target.value)} />
        <Button
          type="submit"
          variant="primary"
          size="lg"
          pending={busy}
          idleLabel="create"
          pendingLabel="creating…"
        />
        <Button
          type="button"
          variant="ghost"
          disabled={busy}
          onClick={() => { setOpen(false); setError(null); }}
        >
          cancel
        </Button>
      </div>
      {error && <div className="text-red-400 text-xs">{error}</div>}
    </form>
  );
}
