import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useState } from "react";
import { api, ApiError } from "../api/client";
import { useToast } from "../toast";
import { PendingButton } from "./PendingButton";

export function AppSettings({ onClose }: { onClose: () => void }) {
  const qc = useQueryClient();
  const { push } = useToast();
  const settingsQ = useQuery({ queryKey: ["settings"], queryFn: api.getSettings });
  const harnessesQ = useQuery({ queryKey: ["harnesses"], queryFn: api.listHarnesses });

  const [harness, setHarness] = useState<string>("");

  useEffect(() => {
    if (settingsQ.data) setHarness(settingsQ.data.harness);
  }, [settingsQ.data]);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onClose]);

  const updateMut = useMutation({
    mutationFn: () => api.updateSettings({ harness }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["settings"] });
      push("success", "Settings saved.");
      onClose();
    },
    onError: (err) => {
      const msg = err instanceof ApiError ? err.message : err instanceof Error ? err.message : String(err);
      push("error", msg);
    },
  });

  const dirty = settingsQ.data ? harness !== settingsQ.data.harness : false;
  const busy = updateMut.isPending;
  const harnesses = harnessesQ.data ?? [];

  return (
    <div className="fixed inset-0 z-40 flex items-center justify-center" role="dialog" aria-modal="true">
      <div className="absolute inset-0 bg-black/50" onClick={busy ? undefined : onClose} />
      <div className="relative w-[520px] max-w-[calc(100vw-2rem)] rounded border border-zinc-800 bg-zinc-950 shadow-lg">
        <header className="flex items-center justify-between border-b border-zinc-800 px-4 py-2">
          <h2 className="text-sm font-semibold">Settings</h2>
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
              disabled={!settingsQ.data || harnessesQ.isLoading}
            >
              {harnesses.length === 0 && settingsQ.data ? (
                <option value={settingsQ.data.harness}>{settingsQ.data.harness}</option>
              ) : null}
              {harnesses.map((h) => (
                <option key={h.id} value={h.id}>
                  {h.label}
                </option>
              ))}
            </select>
            <span className="text-xs text-zinc-500">
              The CLI to launch in each session's terminal. Takes effect on the next session
              attach; running terminals keep their current process.
            </span>
          </label>
          <div className="mt-2 flex items-center justify-end gap-2">
            <button
              type="button"
              className="text-zinc-400 hover:text-zinc-100 disabled:opacity-50"
              onClick={onClose}
              disabled={busy}
            >
              cancel
            </button>
            <PendingButton
              type="submit"
              disabled={!dirty || busy}
              className="rounded bg-zinc-700 px-3 py-1 text-white disabled:opacity-50"
              pending={updateMut.isPending}
              idleLabel="save"
              pendingLabel="saving…"
            />
          </div>
        </form>
      </div>
    </div>
  );
}
