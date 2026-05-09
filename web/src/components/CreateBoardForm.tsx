import { useMutation } from "@tanstack/react-query";
import { api, Board } from "@/api/client";
import { useBoardForm } from "@/hooks/useBoardForm";
import { Button } from "./Button";
import { FormField, FormInput } from "./FormField";
import { Modal } from "./Modal";

export function CreateBoardModal({
  open,
  onClose,
  onCreated,
}: {
  open: boolean;
  onClose: () => void;
  onCreated: (b: Board) => void;
}) {
  const { fields, update, reset, hasMount } = useBoardForm();

  const createMut = useMutation({
    mutationFn: () =>
      api.createBoard({
        name: fields.name,
        mount_path: fields.mount.trim(),
      }),
    onSuccess: (board) => {
      onCreated(board);
      onClose();
      reset();
    },
  });

  const handleClose = () => {
    if (createMut.isPending) return;
    onClose();
  };

  const canSubmit = fields.name.trim() !== "" && hasMount;

  return (
    <Modal open={open} onClose={handleClose} title="New board" busy={createMut.isPending}>
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
            onClick={handleClose}
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
  );
}
