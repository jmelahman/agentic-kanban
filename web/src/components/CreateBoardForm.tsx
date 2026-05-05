import { useMutation } from "@tanstack/react-query";
import { useState } from "react";
import { api, Board } from "@/api/client";
import { Button } from "./Button";
import { Modal } from "./Modal";

export function CreateBoardForm({ onCreated }: { onCreated: (b: Board) => void }) {
  const [open, setOpen] = useState(false);
  const [name, setName] = useState("");
  const [repo, setRepo] = useState("");
  const [mount, setMount] = useState("");
  const [base, setBase] = useState("main");

  const createMut = useMutation({
    mutationFn: () =>
      api.createBoard({
        name,
        repo_path: repo.trim() || undefined,
        mount_path: mount.trim() || undefined,
        base_branch: repo.trim() ? base : undefined,
      }),
    onSuccess: (board) => {
      onCreated(board);
      setOpen(false);
      setName("");
      setRepo("");
      setMount("");
      setBase("main");
    },
  });

  const close = () => {
    if (createMut.isPending) return;
    setOpen(false);
  };

  const hasRepo = repo.trim() !== "";
  const hasMount = mount.trim() !== "";
  const canSubmit = name.trim() !== "" && (hasRepo || hasMount);

  return (
    <>
      <Button
        variant="neutral"
        size="sm"
        className="inline-flex h-7 items-center justify-center"
        onClick={() => setOpen(true)}
      >
        + new board
      </Button>
      <Modal open={open} onClose={close} title="New board" busy={createMut.isPending}>
        <form
          className="flex flex-col gap-3 p-4 text-sm"
          onSubmit={(e) => {
            e.preventDefault();
            if (!canSubmit) return;
            createMut.mutate();
          }}
        >
          <label className="flex flex-col gap-1">
            <span className="text-xs text-fg-muted">Name</span>
            <input
              className="rounded bg-surface px-2 py-1"
              placeholder="name"
              value={name}
              onChange={(e) => setName(e.target.value)}
              required
              autoFocus
            />
          </label>
          <label className="flex flex-col gap-1">
            <span className="text-xs text-fg-muted">Repository path</span>
            <input
              className="rounded bg-surface px-2 py-1"
              placeholder="/host/path/to/repo (optional)"
              value={repo}
              onChange={(e) => setRepo(e.target.value)}
            />
          </label>
          <label className="flex flex-col gap-1">
            <span className="text-xs text-fg-muted">Mount path</span>
            <input
              className="rounded bg-surface px-2 py-1"
              placeholder="host directory mounted into the container (defaults to repository)"
              value={mount}
              onChange={(e) => setMount(e.target.value)}
            />
          </label>
          {hasRepo && (
            <label className="flex flex-col gap-1">
              <span className="text-xs text-fg-muted">Base branch</span>
              <input
                className="rounded bg-surface px-2 py-1"
                placeholder="base branch"
                value={base}
                onChange={(e) => setBase(e.target.value)}
              />
            </label>
          )}
          {!hasRepo && !hasMount && (
            <p className="text-xs text-fg-muted">Provide at least a repository path or a mount path.</p>
          )}
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
              disabled={!canSubmit}
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
