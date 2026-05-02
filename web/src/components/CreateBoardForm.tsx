import { useMutation } from "@tanstack/react-query";
import { useState } from "react";
import { api, Board } from "@/api/client";
import { Button } from "./Button";
import { Modal } from "./Modal";

export function CreateBoardForm({ onCreated }: { onCreated: (b: Board) => void }) {
  const [open, setOpen] = useState(false);
  const [name, setName] = useState("");
  const [repo, setRepo] = useState("");
  const [base, setBase] = useState("main");

  const createMut = useMutation({
    mutationFn: () => api.createBoard({ name, source_repo_path: repo, base_branch: base }),
    onSuccess: (board) => {
      onCreated(board);
      setOpen(false);
      setName("");
      setRepo("");
      setBase("main");
    },
  });

  const close = () => {
    if (createMut.isPending) return;
    setOpen(false);
  };

  return (
    <>
      <Button variant="neutral" size="lg" className="text-sm" onClick={() => setOpen(true)}>
        + new board
      </Button>
      <Modal open={open} onClose={close} title="New board" busy={createMut.isPending}>
        <form
          className="flex flex-col gap-3 p-4 text-sm"
          onSubmit={(e) => {
            e.preventDefault();
            createMut.mutate();
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
          <div className="mt-2 flex items-center justify-end gap-2">
            <Button
              type="button"
              variant="ghost"
              disabled={createMut.isPending}
              onClick={close}
            >
              cancel
            </Button>
            <Button
              type="submit"
              variant="primary"
              size="lg"
              pending={createMut.isPending}
              idleLabel="create"
              pendingLabel="creating…"
            />
          </div>
        </form>
      </Modal>
    </>
  );
}
