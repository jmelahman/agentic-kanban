import { useMutation } from "@tanstack/react-query";
import { useState } from "react";
import { api, Board } from "@/api/client";
import { useBoardForm } from "@/hooks/useBoardForm";
import { PlusIcon } from "@/icons";
import { Button } from "./Button";
import { FormField, FormInput } from "./FormField";
import { Modal } from "./Modal";

export function CreateBoardForm({ onCreated }: { onCreated: (b: Board) => void }) {
  const [open, setOpen] = useState(false);
  const { fields, update, reset, hasMount } = useBoardForm();

  const createMut = useMutation({
    mutationFn: () =>
      api.createBoard({
        name: fields.name,
        mount_path: fields.mount.trim(),
      }),
    onSuccess: (board) => {
      onCreated(board);
      setOpen(false);
      reset();
    },
  });

  const close = () => {
    if (createMut.isPending) return;
    setOpen(false);
  };

  const canSubmit = fields.name.trim() !== "" && hasMount;

  return (
    <>
      <Button
        variant="neutral"
        size="icon"
        className="gap-1 sm:w-auto sm:px-2 sm:text-xs"
        onClick={() => setOpen(true)}
        aria-label="New board"
        title="New board"
      >
        <PlusIcon />
        <span className="hidden sm:inline">new board</span>
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
          <FormField label="Name">
            <FormInput
              value={fields.name}
              onChange={(e) => update("name", e.target.value)}
              required
              autoFocus
            />
          </FormField>
          <FormField
            label="Mount path"
            hint="Working directory agent sessions start from."
          >
            <FormInput
              mono
              placeholder="/host/path/to/folder"
              value={fields.mount}
              onChange={(e) => update("mount", e.target.value)}
              required
            />
          </FormField>
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
