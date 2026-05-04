import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useState } from "react";
import { api } from "@/api/client";
import { queryKeys } from "@/api/keys";
import {
  setTerminalOrientation,
  TerminalOrientation,
  useTerminalOrientation,
} from "@/hooks/useTerminalOrientation";
import { useToast } from "@/toast";
import { Button } from "./Button";
import { Modal } from "./Modal";

export function AppSettings({ open, onClose }: { open: boolean; onClose: () => void }) {
  const qc = useQueryClient();
  const { push } = useToast();
  const settingsQ = useQuery({ queryKey: queryKeys.settings, queryFn: api.getSettings, enabled: open });
  const harnessesQ = useQuery({ queryKey: queryKeys.harnesses, queryFn: api.listHarnesses, enabled: open });
  const versionQ = useQuery({ queryKey: queryKeys.version, queryFn: api.getVersion, staleTime: Infinity, enabled: open });

  const [harness, setHarness] = useState<string>("");
  const savedOrientation = useTerminalOrientation();
  const [orientation, setOrientation] = useState<TerminalOrientation>(savedOrientation);

  useEffect(() => {
    if (settingsQ.data) setHarness(settingsQ.data.harness);
  }, [settingsQ.data]);

  useEffect(() => {
    if (open) setOrientation(savedOrientation);
    // Re-sync the form to the persisted value each time the modal opens.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open]);

  const updateMut = useMutation({
    mutationFn: async () => {
      if (orientation !== savedOrientation) setTerminalOrientation(orientation);
      await api.updateSettings({ harness });
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.settings });
      push("success", "Settings saved.");
      onClose();
    },
  });

  const harnessDirty = settingsQ.data ? harness !== settingsQ.data.harness : false;
  const orientationDirty = orientation !== savedOrientation;
  const dirty = harnessDirty || orientationDirty;
  const busy = updateMut.isPending;
  const harnesses = harnessesQ.data ?? [];

  return (
    <Modal open={open} onClose={onClose} title="Settings" busy={busy}>
      <form
        className="flex flex-col gap-3 p-4 text-sm"
        onSubmit={(e) => {
          e.preventDefault();
          if (!dirty) return;
          updateMut.mutate();
        }}
      >
        <label className="flex flex-col gap-1">
          <span className="text-xs text-zinc-400">Agent harness</span>
          <select
            className="rounded bg-zinc-900 px-2 py-1"
            value={harness}
            onChange={(e) => setHarness(e.target.value)}
            disabled={settingsQ.isLoading || harnessesQ.isLoading}
          >
            <option value="">— use project / default —</option>
            {harnesses.map((h) => (
              <option key={h.id} value={h.id}>
                {h.label}
              </option>
            ))}
          </select>
          <span className="text-xs text-zinc-500">
            Saved to <span className="font-mono">~/.config/kanban/config.toml</span>. Takes
            effect on the next session attach; running terminals keep their current process.
            Leave unset to fall back to the repo's <span className="font-mono">.kanban.toml</span>
            {" "}or the default.
          </span>
        </label>
        <label className="flex flex-col gap-1">
          <span className="text-xs text-zinc-400">Terminal position</span>
          <select
            className="rounded bg-zinc-900 px-2 py-1"
            value={orientation}
            onChange={(e) => setOrientation(e.target.value as TerminalOrientation)}
          >
            <option value="vertical">vertical (right side)</option>
            <option value="horizontal">horizontal (bottom)</option>
          </select>
        </label>
        <div className="mt-2 flex items-center justify-end gap-2">
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
            variant="secondary"
            size="lg"
            disabled={!dirty || busy}
            pending={updateMut.isPending}
            idleLabel="save"
            pendingLabel="saving…"
          />
        </div>
      </form>
      <footer className="border-t border-zinc-800 px-4 py-2 font-mono text-[11px] text-zinc-500">
        {versionQ.data?.version ?? "…"}
      </footer>
    </Modal>
  );
}
