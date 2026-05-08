import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useEffect, useState } from "react";
import { api, Board } from "@/api/client";
import { queryKeys } from "@/api/keys";
import { useToast } from "@/toast";
import { Button } from "./Button";
import { Modal } from "./Modal";
import { Tab } from "./Tab";

type BoardSettingsTab = "general" | "git" | "danger";

export function BoardSettings({
  open,
  board,
  onClose,
  onDeleted,
}: {
  open: boolean;
  board: Board;
  onClose: () => void;
  onDeleted: () => void;
}) {
  const qc = useQueryClient();
  const { push } = useToast();
  const [tab, setTab] = useState<BoardSettingsTab>("general");
  const [name, setName] = useState(board.name);
  const [repo, setRepo] = useState(board.repo_path);
  const [mount, setMount] = useState(board.mount_path);
  const [worktreeRoot, setWorktreeRoot] = useState(board.worktree_root);
  const [base, setBase] = useState(board.base_branch);
  const [branchPrefix, setBranchPrefix] = useState(board.branch_prefix);
  const [gitAuthorName, setGitAuthorName] = useState(board.git_author_name);
  const [gitAuthorEmail, setGitAuthorEmail] = useState(board.git_author_email);
  const [confirmDelete, setConfirmDelete] = useState(false);

  useEffect(() => {
    setName(board.name);
    setRepo(board.repo_path);
    setMount(board.mount_path);
    setWorktreeRoot(board.worktree_root);
    setBase(board.base_branch);
    setBranchPrefix(board.branch_prefix);
    setGitAuthorName(board.git_author_name);
    setGitAuthorEmail(board.git_author_email);
  }, [
    board.id,
    board.name,
    board.repo_path,
    board.mount_path,
    board.worktree_root,
    board.base_branch,
    board.branch_prefix,
    board.git_author_name,
    board.git_author_email,
  ]);

  useEffect(() => {
    if (open) setTab("general");
  }, [open]);

  const updateMut = useMutation({
    mutationFn: () =>
      api.updateBoard(board.id, {
        name: name.trim(),
        repo_path: repo.trim(),
        mount_path: mount.trim(),
        worktree_root: worktreeRoot.trim(),
        base_branch: base.trim(),
        branch_prefix: branchPrefix.trim(),
        git_author_name: gitAuthorName.trim(),
        git_author_email: gitAuthorEmail.trim(),
      }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.boards });
      qc.invalidateQueries({ queryKey: queryKeys.board(board.id) });
      push("success", "Board updated.");
      onClose();
    },
  });

  const deleteMut = useMutation({
    mutationFn: () => api.deleteBoard(board.id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.boards });
      push("success", `Deleted board "${board.name}".`);
      onDeleted();
    },
  });

  const dirty =
    name.trim() !== board.name ||
    repo.trim() !== board.repo_path ||
    mount.trim() !== board.mount_path ||
    worktreeRoot.trim() !== board.worktree_root ||
    base.trim() !== board.base_branch ||
    branchPrefix.trim() !== board.branch_prefix ||
    gitAuthorName.trim() !== board.git_author_name ||
    gitAuthorEmail.trim() !== board.git_author_email;
  const hasRepo = repo.trim() !== "";
  const hasMount = mount.trim() !== "";
  const valid =
    name.trim() !== "" &&
    (hasRepo || hasMount) &&
    (!hasRepo || base.trim() !== "") &&
    (!hasRepo || worktreeRoot.trim() !== "");
  const worktreeRootChanged = worktreeRoot.trim() !== board.worktree_root;
  const busy = updateMut.isPending || deleteMut.isPending;

  return (
    <Modal open={open} onClose={onClose} title="Board settings" busy={busy}>
      <div className="flex border-b border-border text-sm">
        <Tab active={tab === "general"} onClick={() => setTab("general")} label="general" />
        <Tab active={tab === "git"} onClick={() => setTab("git")} label="git" />
        <Tab active={tab === "danger"} onClick={() => setTab("danger")} label="danger" />
      </div>
      <form
        className="flex min-h-[420px] flex-col gap-3 p-4 text-sm"
        onSubmit={(e) => {
          e.preventDefault();
          if (!dirty || !valid) return;
          updateMut.mutate();
        }}
      >
        {tab === "general" && (
          <>
            <label className="flex flex-col gap-1">
              <span className="text-xs text-fg-muted">Name</span>
              <input
                className="rounded bg-surface px-2 py-1"
                value={name}
                onChange={(e) => setName(e.target.value)}
                required
              />
            </label>
            <label className="flex flex-col gap-1">
              <span className="text-xs text-fg-muted">Mount path</span>
              <input
                className="rounded bg-surface px-2 py-1 font-mono"
                placeholder="host directory mounted into the container (defaults to repository)"
                value={mount}
                onChange={(e) => setMount(e.target.value)}
              />
            </label>
            <div className="flex flex-col gap-1 text-xs text-fg-muted">
              <span>slug: <span className="font-mono">{board.slug}</span></span>
            </div>
          </>
        )}
        {tab === "git" && (
          <>
            <label className="flex flex-col gap-1">
              <span className="text-xs text-fg-muted">Repository path</span>
              <input
                className="rounded bg-surface px-2 py-1 font-mono"
                placeholder="/host/path/to/repo (optional)"
                value={repo}
                onChange={(e) => setRepo(e.target.value)}
              />
            </label>
            {hasRepo && (
              <label className="flex flex-col gap-1">
                <span className="text-xs text-fg-muted">Base branch</span>
                <input
                  className="rounded bg-surface px-2 py-1"
                  value={base}
                  onChange={(e) => setBase(e.target.value)}
                  required
                />
              </label>
            )}
            {hasRepo && (
              <label className="flex flex-col gap-1">
                <span className="text-xs text-fg-muted">Worktree root</span>
                <input
                  className="rounded bg-surface px-2 py-1 font-mono"
                  placeholder="parent directory for this board's worktrees"
                  value={worktreeRoot}
                  onChange={(e) => setWorktreeRoot(e.target.value)}
                  required
                />
                {worktreeRootChanged && (
                  <span className="text-xs text-amber-400">
                    Existing sessions will keep their old worktree paths and won't be
                    cleaned up automatically when the board is deleted. Stop and destroy
                    them first if you want a clean switch.
                  </span>
                )}
              </label>
            )}
            {hasRepo && (
              <label className="flex flex-col gap-1">
                <span className="text-xs text-fg-muted">Branch prefix</span>
                <input
                  className="rounded bg-surface px-2 py-1 font-mono"
                  placeholder={`kanban/${board.slug}`}
                  value={branchPrefix}
                  onChange={(e) => setBranchPrefix(e.target.value)}
                />
                <span className="text-xs text-fg-muted">
                  New session branches are{" "}
                  <span className="font-mono">
                    {(branchPrefix.trim() || `kanban/${board.slug}`) + "/<ticket-slug>"}
                  </span>
                  . Existing sessions keep their original branch.
                </span>
              </label>
            )}
            {hasRepo && (
              <fieldset className="flex flex-col gap-2 rounded border border-border p-3">
                <legend className="px-1 text-xs text-fg-muted">Commit identity</legend>
                <label className="flex flex-col gap-1">
                  <span className="text-xs text-fg-muted">Author name</span>
                  <input
                    className="rounded bg-surface px-2 py-1"
                    placeholder="leave blank to use the kanban container's git config"
                    value={gitAuthorName}
                    onChange={(e) => setGitAuthorName(e.target.value)}
                  />
                </label>
                <label className="flex flex-col gap-1">
                  <span className="text-xs text-fg-muted">Author email</span>
                  <input
                    type="email"
                    className="rounded bg-surface px-2 py-1 font-mono"
                    placeholder="you@example.com"
                    value={gitAuthorEmail}
                    onChange={(e) => setGitAuthorEmail(e.target.value)}
                  />
                </label>
                <span className="text-xs text-fg-muted">
                  Used for merge/squash commits when finishing a session. Set both
                  fields to override; leave blank to fall back to whatever git can
                  auto-detect inside the kanban container.
                </span>
              </fieldset>
            )}
          </>
        )}
        {tab === "danger" && (
          <div>
            <h3 className="text-xs font-semibold uppercase tracking-wide text-danger">Danger zone</h3>
            <p className="mt-1 text-xs text-fg-muted">
              Deletes this board, all its tickets, and stops/destroys every running session
              (containers, worktrees, and branches).
            </p>
            <Button
              type="button"
              variant="danger"
              size="lg"
              className="mt-2 text-xs"
              disabled={busy}
              onClick={() => setConfirmDelete(true)}
              pending={deleteMut.isPending}
              idleLabel="delete board"
              pendingLabel="deleting…"
            />
          </div>
        )}
        {tab !== "danger" && (
          <div className="mt-auto flex items-center justify-end gap-2">
            <Button
              type="button"
              variant="ghost"
              onClick={onClose}
              disabled={busy}
            >
              cancel
            </Button>
            <Button
              type="submit"
              variant="primary"
              size="lg"
              disabled={!dirty || !valid || busy}
              pending={updateMut.isPending}
              idleLabel="save"
              pendingLabel="saving…"
            />
          </div>
        )}
      </form>
      <Modal
        open={confirmDelete}
        onClose={() => setConfirmDelete(false)}
        title="Permanently delete board?"
        busy={deleteMut.isPending}
      >
        <div className="p-4">
          <p className="text-sm">
            Permanently delete board <span className="font-medium">"{board.name}"</span>?
          </p>
          <p className="mt-2 text-xs text-fg-muted">
            This stops all containers, removes worktrees, deletes branches, and removes
            every ticket.
          </p>
          <div className="mt-4 flex justify-end gap-2">
            <Button
              type="button"
              variant="ghost"
              onClick={() => setConfirmDelete(false)}
              disabled={deleteMut.isPending}
            >
              cancel
            </Button>
            <Button
              type="button"
              variant="danger"
              size="lg"
              onClick={() => deleteMut.mutate()}
              disabled={deleteMut.isPending}
              pending={deleteMut.isPending}
              idleLabel="delete board"
              pendingLabel="deleting…"
            />
          </div>
        </div>
      </Modal>
    </Modal>
  );
}
