import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useEffect, useState } from "react";
import { api, Board } from "@/api/client";
import { queryKeys } from "@/api/keys";
import { useBoardForm } from "@/hooks/useBoardForm";
import { useToast } from "@/toast";
import { Button } from "./Button";
import { FormField, FormInput } from "./FormField";
import { Modal } from "./Modal";
import { Tab } from "./Tab";

type BoardSettingsTab = "general" | "git" | "danger";

function fieldsFromBoard(board: Board) {
  return {
    name: board.name,
    repo: board.repo_path,
    mount: board.mount_path,
    worktreeRoot: board.worktree_root,
    base: board.base_branch,
    branchPrefix: board.branch_prefix,
    gitAuthorName: board.git_author_name,
    gitAuthorEmail: board.git_author_email,
  };
}

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
  const { fields, update, reset, hasRepo, hasMount } = useBoardForm(fieldsFromBoard(board));
  const [confirmDelete, setConfirmDelete] = useState(false);

  useEffect(() => {
    reset(fieldsFromBoard(board));
  }, [
    reset,
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
        name: fields.name.trim(),
        repo_path: fields.repo.trim(),
        mount_path: fields.mount.trim(),
        worktree_root: fields.worktreeRoot.trim(),
        base_branch: fields.base.trim(),
        branch_prefix: fields.branchPrefix.trim(),
        git_author_name: fields.gitAuthorName.trim(),
        git_author_email: fields.gitAuthorEmail.trim(),
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
    fields.name.trim() !== board.name ||
    fields.repo.trim() !== board.repo_path ||
    fields.mount.trim() !== board.mount_path ||
    fields.worktreeRoot.trim() !== board.worktree_root ||
    fields.base.trim() !== board.base_branch ||
    fields.branchPrefix.trim() !== board.branch_prefix ||
    fields.gitAuthorName.trim() !== board.git_author_name ||
    fields.gitAuthorEmail.trim() !== board.git_author_email;
  const valid =
    fields.name.trim() !== "" &&
    (hasRepo || hasMount) &&
    (!hasRepo || fields.base.trim() !== "") &&
    (!hasRepo || fields.worktreeRoot.trim() !== "");
  const worktreeRootChanged = fields.worktreeRoot.trim() !== board.worktree_root;
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
            <FormField label="Name">
              <FormInput
                value={fields.name}
                onChange={(e) => update("name", e.target.value)}
                required
              />
            </FormField>
            <FormField label="Mount path">
              <FormInput
                mono
                placeholder="host directory mounted into the container (defaults to repository)"
                value={fields.mount}
                onChange={(e) => update("mount", e.target.value)}
              />
            </FormField>
            <div className="flex flex-col gap-1 text-xs text-fg-muted">
              <span>slug: <span className="font-mono">{board.slug}</span></span>
            </div>
          </>
        )}
        {tab === "git" && (
          <>
            <FormField label="Repository path">
              <FormInput
                mono
                placeholder="/host/path/to/repo (optional)"
                value={fields.repo}
                onChange={(e) => update("repo", e.target.value)}
              />
            </FormField>
            {hasRepo && (
              <FormField label="Base branch">
                <FormInput
                  value={fields.base}
                  onChange={(e) => update("base", e.target.value)}
                  required
                />
              </FormField>
            )}
            {hasRepo && (
              <FormField label="Worktree root">
                <FormInput
                  mono
                  placeholder="parent directory for this board's worktrees"
                  value={fields.worktreeRoot}
                  onChange={(e) => update("worktreeRoot", e.target.value)}
                  required
                />
                {worktreeRootChanged && (
                  <span className="text-xs text-amber-400">
                    Existing sessions will keep their old worktree paths and won't be
                    cleaned up automatically when the board is deleted. Stop and destroy
                    them first if you want a clean switch.
                  </span>
                )}
              </FormField>
            )}
            {hasRepo && (
              <FormField label="Branch prefix">
                <FormInput
                  mono
                  placeholder={`kanban/${board.slug}`}
                  value={fields.branchPrefix}
                  onChange={(e) => update("branchPrefix", e.target.value)}
                />
                <span className="text-xs text-fg-muted">
                  New session branches are{" "}
                  <span className="font-mono">
                    {(fields.branchPrefix.trim() || `kanban/${board.slug}`) + "/<ticket-slug>"}
                  </span>
                  . Existing sessions keep their original branch.
                </span>
              </FormField>
            )}
            {hasRepo && (
              <fieldset className="flex flex-col gap-2 rounded border border-border p-3">
                <legend className="px-1 text-xs text-fg-muted">Commit identity</legend>
                <FormField label="Author name">
                  <FormInput
                    placeholder="leave blank to use the kanban container's git config"
                    value={fields.gitAuthorName}
                    onChange={(e) => update("gitAuthorName", e.target.value)}
                  />
                </FormField>
                <FormField label="Author email">
                  <FormInput
                    mono
                    type="email"
                    placeholder="you@example.com"
                    value={fields.gitAuthorEmail}
                    onChange={(e) => update("gitAuthorEmail", e.target.value)}
                  />
                </FormField>
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
