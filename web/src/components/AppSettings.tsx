import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useState } from "react";
import { api } from "@/api/client";
import { queryKeys } from "@/api/keys";
import { ACCENTS, type Accent, setAccent, useAccent } from "@/hooks/useAccent";
import { type Contrast, setContrast, useContrast } from "@/hooks/useContrast";
import { setThemeMode, type ThemeMode, useThemeMode } from "@/hooks/useThemeMode";
import {
  setTerminalOrientation,
  type TerminalOrientation,
  useTerminalOrientation,
} from "@/hooks/useTerminalOrientation";
import { useToast } from "@/toast";
import { Button } from "./Button";
import { FormField, FormInput } from "./FormField";
import { KeybindingsSettings } from "./KeybindingsSettings";
import { Modal } from "./Modal";
import { Tab } from "./Tab";

const ACCENT_LABELS: Record<Accent, string> = {
  red: "Red",
  orange: "Orange",
  amber: "Amber",
  green: "Green",
  teal: "Teal",
  blue: "Blue",
  indigo: "Indigo",
  purple: "Purple",
  pink: "Pink",
};

type SettingsTab = "general" | "appearance" | "shortcuts";

export function AppSettings({ open, onClose }: { open: boolean; onClose: () => void }) {
  const qc = useQueryClient();
  const { push } = useToast();
  const settingsQ = useQuery({
    queryKey: queryKeys.settings,
    queryFn: api.getSettings,
    enabled: open,
  });
  const harnessesQ = useQuery({
    queryKey: queryKeys.harnesses,
    queryFn: api.listHarnesses,
    enabled: open,
  });
  const versionQ = useQuery({
    queryKey: queryKeys.version,
    queryFn: api.getVersion,
    staleTime: Infinity,
    enabled: open,
  });

  const [tab, setTab] = useState<SettingsTab>("general");
  const [harness, setHarness] = useState<string>("");
  const [worktreesRoot, setWorktreesRoot] = useState<string>("");
  const savedOrientation = useTerminalOrientation();
  const [orientation, setOrientation] = useState<TerminalOrientation>(savedOrientation);
  const { mode } = useThemeMode();
  const contrast = useContrast();
  const accent = useAccent();

  useEffect(() => {
    if (settingsQ.data) {
      setHarness(settingsQ.data.harness);
      setWorktreesRoot(settingsQ.data.worktrees_root);
    }
  }, [settingsQ.data]);

  useEffect(() => {
    if (open) {
      setOrientation(savedOrientation);
      setTab("general");
    }
    // Re-sync the form to the persisted value each time the modal opens.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open]);

  const updateMut = useMutation({
    mutationFn: async () => {
      if (orientation !== savedOrientation) setTerminalOrientation(orientation);
      const payload: { harness?: string; worktrees_root?: string } = {};
      if (settingsQ.data && harness !== settingsQ.data.harness) payload.harness = harness;
      if (
        settingsQ.data &&
        !settingsQ.data.worktrees_root_locked &&
        worktreesRoot.trim() !== settingsQ.data.worktrees_root
      ) {
        payload.worktrees_root = worktreesRoot.trim();
      }
      await api.updateSettings(payload);
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.settings });
      push("success", "Settings saved.");
      onClose();
    },
  });

  const harnessDirty = settingsQ.data ? harness !== settingsQ.data.harness : false;
  const worktreesRootDirty = settingsQ.data
    ? !settingsQ.data.worktrees_root_locked &&
      worktreesRoot.trim() !== settingsQ.data.worktrees_root
    : false;
  const orientationDirty = orientation !== savedOrientation;
  const dirty = harnessDirty || orientationDirty || worktreesRootDirty;
  const busy = updateMut.isPending;
  const harnesses = harnessesQ.data ?? [];
  const worktreesLocked = settingsQ.data?.worktrees_root_locked ?? false;
  const worktreesResolved = settingsQ.data?.worktrees_root_resolved ?? "";

  return (
    <Modal open={open} onClose={onClose} title="Settings" busy={busy}>
      <div className="flex border-b border-border text-sm">
        <Tab active={tab === "general"} onClick={() => setTab("general")} label="general" />
        <Tab
          active={tab === "appearance"}
          onClick={() => setTab("appearance")}
          label="appearance"
        />
        <Tab active={tab === "shortcuts"} onClick={() => setTab("shortcuts")} label="shortcuts" />
      </div>
      <form
        className="flex min-h-[420px] flex-col gap-3 p-4 text-sm"
        onSubmit={(e) => {
          e.preventDefault();
          if (!dirty) return;
          updateMut.mutate();
        }}
      >
        {tab === "general" && (
          <>
            <FormField label="Agent harness">
              <select
                className="rounded bg-surface px-2 py-1"
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
              <span className="text-xs text-fg-muted">
                Saved to <span className="font-mono">~/.config/kanban/config.toml</span>. Takes
                effect on the next session attach; running terminals keep their current process.
                Leave unset to fall back to the repo's{" "}
                <span className="font-mono">.kanban.toml</span> or the default.
              </span>
            </FormField>
            <FormField label="Worktrees directory">
              <FormInput
                mono
                type="text"
                className="disabled:opacity-50"
                value={worktreesRoot}
                placeholder={worktreesResolved || "~/.local/share/kanban/worktrees"}
                onChange={(e) => setWorktreesRoot(e.target.value)}
                disabled={settingsQ.isLoading || worktreesLocked}
                spellCheck={false}
              />
              <span className="text-xs text-fg-muted">
                Parent directory for new boards' worktrees. Leave empty to use the default. Supports{" "}
                <span className="font-mono">~</span> for your home directory. Existing boards keep
                their stored <span className="font-mono">worktree_root</span>.
                {worktreesLocked && (
                  <>
                    {" "}
                    Currently locked by <span className="font-mono">--worktrees-dir</span> or{" "}
                    <span className="font-mono">$KANBAN_WORKTREES_DIR</span>:{" "}
                    <span className="font-mono">{worktreesResolved}</span>.
                  </>
                )}
              </span>
            </FormField>
            <FormField label="Terminal position">
              <select
                className="rounded bg-surface px-2 py-1"
                value={orientation}
                onChange={(e) => setOrientation(e.target.value as TerminalOrientation)}
              >
                <option value="vertical">right</option>
                <option value="horizontal">bottom</option>
              </select>
            </FormField>
          </>
        )}
        {tab === "appearance" && (
          <fieldset className="flex flex-col gap-2">
            <div className="flex flex-col gap-1">
              <span className="text-xs text-fg-muted">Theme</span>
              <ThemeModeToggle value={mode} onChange={setThemeMode} />
            </div>
            <label className="flex items-center gap-2">
              <input
                type="checkbox"
                checked={contrast === "high"}
                onChange={(e) => setContrast(e.target.checked ? "high" : ("normal" as Contrast))}
              />
              <span>High contrast</span>
            </label>
            <div className="flex flex-col gap-1">
              <span className="text-xs text-fg-muted">Accent</span>
              <AccentSwatches value={accent} onChange={setAccent} />
            </div>
          </fieldset>
        )}
        {tab === "shortcuts" && <KeybindingsSettings />}
        <div className="mt-auto flex items-center justify-end gap-2">
          <Button type="button" variant="ghost" onClick={onClose} disabled={busy}>
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
      <footer className="border-t border-border px-4 py-2 font-mono text-[11px] text-fg-muted">
        {versionQ.data?.version ?? "…"}
      </footer>
    </Modal>
  );
}

function ThemeModeToggle({
  value,
  onChange,
}: {
  value: ThemeMode;
  onChange: (v: ThemeMode) => void;
}) {
  const opts: { v: ThemeMode; label: string }[] = [
    { v: "system", label: "System" },
    { v: "light", label: "Light" },
    { v: "dark", label: "Dark" },
  ];
  return (
    <div
      role="radiogroup"
      className="inline-flex w-fit overflow-hidden rounded border border-border"
    >
      {opts.map((o, i) => {
        const active = o.v === value;
        return (
          <button
            key={o.v}
            type="button"
            role="radio"
            aria-checked={active}
            onClick={() => onChange(o.v)}
            className={`px-3 py-1 text-sm transition-colors duration-150 ${
              i > 0 ? "border-l border-border" : ""
            } ${active ? "bg-accent-700 text-white" : "bg-surface text-fg hover:bg-surface-2"}`}
          >
            {o.label}
          </button>
        );
      })}
    </div>
  );
}

function AccentSwatches({ value, onChange }: { value: Accent; onChange: (v: Accent) => void }) {
  return (
    <div className="flex flex-wrap gap-2">
      {ACCENTS.map((a) => {
        const active = a === value;
        return (
          <button
            key={a}
            type="button"
            data-accent-swatch={a}
            aria-label={ACCENT_LABELS[a]}
            aria-pressed={active}
            title={ACCENT_LABELS[a]}
            onClick={() => onChange(a)}
            data-accent={a}
            className={`h-7 w-7 rounded-full border border-border transition-transform duration-150 hover:scale-110 ${
              active ? "ring-2 ring-fg ring-offset-2 ring-offset-surface" : ""
            }`}
            style={{ backgroundColor: "var(--color-accent-700)" }}
          />
        );
      })}
    </div>
  );
}
