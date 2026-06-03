import { useIsFetching, useIsMutating } from "@tanstack/react-query";
import { type ReactNode, useState } from "react";
import type { SseStatus } from "@/api/runtimeSignals";
import { GaugeIcon, XIcon } from "@/icons";
import {
  formatBytes,
  useFrameStats,
  useInflightRequests,
  useRuntimeSamples,
  useSseStatus,
} from "./metrics";
import {
  type DevToolbarCorner,
  type DevToolbarSectionKey,
  DEV_TOOLBAR_CORNERS,
  setDevToolbarPrefs,
  useDevToolbarPrefs,
} from "./preferences";

const CORNER_CLASS: Record<DevToolbarCorner, string> = {
  // top-14 clears the ~44px header so a top-pinned widget doesn't overlap it.
  br: "bottom-3 right-3",
  bl: "bottom-3 left-3",
  tr: "top-14 right-3",
  tl: "top-14 left-3",
};

const SECTION_LABELS: Record<DevToolbarSectionKey, string> = {
  fps: "FPS",
  memory: "Memory",
  domReact: "DOM / React",
  appInternals: "App internals",
};

const SECTION_ORDER: DevToolbarSectionKey[] = ["fps", "memory", "domReact", "appInternals"];

const SSE_TONE: Record<SseStatus, string> = {
  open: "text-emerald-400",
  error: "text-amber-400",
  closed: "text-fg-muted",
};

// DevToolbar is an opt-in corner overlay of live frontend health metrics. It is
// only mounted when the `dev_toolbar.enabled` config flag is on AND the user has
// toggled it open, so its samplers never run for normal users.
export function DevToolbar() {
  const prefs = useDevToolbarPrefs();
  const [showConfig, setShowConfig] = useState(false);

  const frame = useFrameStats(prefs.sections.fps);
  const samples = useRuntimeSamples(prefs.sections.memory || prefs.sections.domReact);
  const isFetching = useIsFetching();
  const isMutating = useIsMutating();
  const inflight = useInflightRequests();
  const sse = useSseStatus();

  const close = () => setDevToolbarPrefs({ ...prefs, open: false });
  const cycleCorner = () => {
    const idx = DEV_TOOLBAR_CORNERS.indexOf(prefs.corner);
    const next = DEV_TOOLBAR_CORNERS[(idx + 1) % DEV_TOOLBAR_CORNERS.length];
    setDevToolbarPrefs({ ...prefs, corner: next });
  };
  const toggleSection = (key: DevToolbarSectionKey) =>
    setDevToolbarPrefs({
      ...prefs,
      sections: { ...prefs.sections, [key]: !prefs.sections[key] },
    });

  return (
    <div
      className={`fixed z-50 w-44 select-none rounded border border-border bg-surface-2/95 font-mono text-[11px] text-fg shadow-lg backdrop-blur ${CORNER_CLASS[prefs.corner]}`}
    >
      <div className="flex items-center gap-1 border-b border-border px-2 py-1">
        <GaugeIcon size={12} className="text-fg-muted" />
        <span className="font-semibold tracking-wide text-fg-muted">dev</span>
        <div className="ml-auto flex items-center gap-1">
          <HeaderButton
            label="Configure metrics"
            active={showConfig}
            onClick={() => setShowConfig((v) => !v)}
          >
            ⚙
          </HeaderButton>
          <HeaderButton label="Move to next corner" onClick={cycleCorner}>
            ⤢
          </HeaderButton>
          <HeaderButton label="Close developer toolbar" onClick={close}>
            <XIcon size={12} />
          </HeaderButton>
        </div>
      </div>

      {showConfig && (
        <div className="border-b border-border px-2 py-1">
          {SECTION_ORDER.map((key) => (
            <label
              key={key}
              className="flex cursor-pointer items-center gap-2 py-0.5 text-fg-muted hover:text-fg"
            >
              <input
                type="checkbox"
                className="accent-accent-500"
                checked={prefs.sections[key]}
                onChange={() => toggleSection(key)}
              />
              {SECTION_LABELS[key]}
            </label>
          ))}
        </div>
      )}

      <div className="py-1">
        {prefs.sections.fps && (
          <Section title={SECTION_LABELS.fps}>
            <Row label="fps" value={frame.fps || "—"} warn={frame.fps > 0 && frame.fps < 30} />
            <Row label="worst" value={`${frame.worstMs} ms`} warn={frame.worstMs > 50} />
          </Section>
        )}

        {prefs.sections.memory && (
          <Section title={SECTION_LABELS.memory}>
            <Row label="heap" value={formatBytes(samples.heapUsed)} />
            <Row label="total" value={formatBytes(samples.heapTotal)} />
            <Row label="limit" value={formatBytes(samples.heapLimit)} />
          </Section>
        )}

        {prefs.sections.domReact && (
          <Section title={SECTION_LABELS.domReact}>
            <Row label="dom nodes" value={samples.domNodes.toLocaleString()} />
            <Row label="queries" value={samples.queryCacheCount} />
            <Row label="fetching" value={isFetching} warn={isFetching > 0} />
            <Row label="mutating" value={isMutating} warn={isMutating > 0} />
          </Section>
        )}

        {prefs.sections.appInternals && (
          <Section title={SECTION_LABELS.appInternals}>
            <Row label="sse" value={<span className={SSE_TONE[sse]}>{sse}</span>} />
            <Row label="in-flight" value={inflight} warn={inflight > 0} />
          </Section>
        )}

        {SECTION_ORDER.every((k) => !prefs.sections[k]) && (
          <p className="px-2 py-1 text-fg-muted">No metrics enabled — click ⚙.</p>
        )}
      </div>
    </div>
  );
}

function HeaderButton({
  label,
  active,
  onClick,
  children,
}: {
  label: string;
  active?: boolean;
  onClick: () => void;
  children: ReactNode;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      aria-label={label}
      title={label}
      className={`inline-flex h-5 w-5 items-center justify-center rounded text-fg-muted hover:bg-surface-3 hover:text-fg ${
        active ? "bg-surface-3 text-fg" : ""
      }`}
    >
      {children}
    </button>
  );
}

function Section({ title, children }: { title: string; children: ReactNode }) {
  return (
    <div className="px-2 py-0.5">
      <div className="text-[10px] uppercase tracking-wider text-fg-muted/70">{title}</div>
      {children}
    </div>
  );
}

function Row({ label, value, warn }: { label: string; value: ReactNode; warn?: boolean }) {
  return (
    <div className="flex items-center justify-between gap-2 leading-tight">
      <span className="text-fg-muted">{label}</span>
      <span className={warn ? "text-amber-400" : "text-fg"}>{value}</span>
    </div>
  );
}
