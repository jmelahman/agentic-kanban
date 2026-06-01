import { parsePatchFiles, type FileDiffMetadata } from "@pierre/diffs";
import { FileDiff } from "@pierre/diffs/react";
import { useQuery } from "@tanstack/react-query";
import { memo, useCallback, useEffect, useMemo, useRef, useState } from "react";
import { api, type Session } from "@/api/client";
import { queryKeys } from "@/api/keys";
import { useContrast } from "@/hooks/useContrast";
import { useThemeMode } from "@/hooks/useThemeMode";

const SIDEBAR_COLLAPSED_KEY = "diff.sidebar.collapsed";

// Re-skin @pierre/diffs to match the app. The library renders into shadow DOM
// and themes itself through `--diffs-*` custom properties; this CSS is injected
// into its top-priority `unsafe` layer. Custom properties inherit across the
// shadow boundary, so `var(--color-*)` resolves against the app's own tokens
// and automatically follows the active `data-theme`. Everything the library
// draws (line tints, gutters, separators) is color-mixed from `--diffs-bg`, so
// pinning that to `--color-bg` recolors the whole surface. Syntax token colors
// still come from the Shiki theme passed via `options.theme`.
const DIFF_UNSAFE_CSS = `:host{
  --diffs-bg: var(--color-bg);
  --diffs-font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", "Courier New", monospace;
  --diffs-header-font-family: ui-sans-serif, system-ui, sans-serif;
  --diffs-addition-color: #3fb950;
  --diffs-deletion-color: var(--color-danger);
  --diffs-modified-color: #58a6ff;
}
[data-diffs-header],
[data-diffs-header][data-sticky]{
  background-color: var(--color-surface);
  border-bottom: 1px solid var(--color-border);
}`;

function loadSidebarCollapsed(): boolean {
  try {
    return localStorage.getItem(SIDEBAR_COLLAPSED_KEY) === "1";
  } catch {
    return false;
  }
}

// Per-file change-type marker. The app's token palette has no green/yellow/blue,
// so additions/renames use fixed hexes that read on both light and dark themes;
// deletions reuse the theme-aware `text-danger` token.
function changeMarker(type: FileDiffMetadata["type"]): { label: string; cls: string } {
  switch (type) {
    case "new":
      return { label: "A", cls: "text-[#3fb950]" };
    case "deleted":
      return { label: "D", cls: "text-danger" };
    case "rename-pure":
    case "rename-changed":
      return { label: "R", cls: "text-[#58a6ff]" };
    default:
      return { label: "M", cls: "text-[#d29922]" };
  }
}

function fileStats(f: FileDiffMetadata): { adds: number; dels: number } {
  let adds = 0;
  let dels = 0;
  for (const h of f.hunks) {
    adds += h.additionLines;
    dels += h.deletionLines;
  }
  return { adds, dels };
}

function displayPath(f: FileDiffMetadata): string {
  return f.prevName ? `${f.prevName} → ${f.name}` : f.name;
}

type Leaf = { kind: "file"; name: string; path: string; file: FileDiffMetadata };
type Dir = { kind: "dir"; name: string; path: string; children: TreeNode[] };
type TreeNode = Leaf | Dir;

// buildTree turns the flat list of changed file paths into a nested
// folder/file tree (split on "/"), with directories sorted before files.
function buildTree(files: FileDiffMetadata[]): TreeNode[] {
  const root: Dir = { kind: "dir", name: "", path: "", children: [] };
  for (const file of files) {
    const parts = file.name.split("/");
    let cur = root;
    parts.forEach((part, i) => {
      if (i === parts.length - 1) {
        cur.children.push({ kind: "file", name: part, path: file.name, file });
        return;
      }
      let next = cur.children.find((c): c is Dir => c.kind === "dir" && c.name === part);
      if (!next) {
        next = { kind: "dir", name: part, path: parts.slice(0, i + 1).join("/"), children: [] };
        cur.children.push(next);
      }
      cur = next;
    });
  }
  const sort = (nodes: TreeNode[]) => {
    nodes.sort((a, b) => {
      if (a.kind !== b.kind) return a.kind === "dir" ? -1 : 1;
      return a.name.localeCompare(b.name);
    });
    for (const n of nodes) if (n.kind === "dir") sort(n.children);
  };
  sort(root.children);
  return root.children;
}

// flattenLeaves walks the sorted tree depth-first so the stacked diffs render
// in the same order the sidebar lists them.
function flattenLeaves(nodes: TreeNode[], out: FileDiffMetadata[] = []): FileDiffMetadata[] {
  for (const n of nodes) {
    if (n.kind === "file") out.push(n.file);
    else flattenLeaves(n.children, out);
  }
  return out;
}

export function DiffPanel({ session }: { session: Session }) {
  const { resolved } = useThemeMode();
  // Syntax colors come from the Shiki theme; the surrounding surface, gutters,
  // borders and accents are remapped to app tokens in DIFF_UNSAFE_CSS, which
  // already track `data-contrast` because the tokens themselves do. Pick the
  // high-contrast Shiki variant so the code text is HC-tuned to match.
  const highContrast = useContrast() === "high";
  const theme = `github-${resolved}${highContrast ? "-high-contrast" : ""}`;

  const diffQ = useQuery({
    queryKey: queryKeys.sessionDiff(session.id),
    queryFn: () => api.getSessionDiff(session.id),
    refetchInterval: 4000,
  });

  // The panel only mounts while the diff tab is active, so polling is cheap.
  const files = useMemo<FileDiffMetadata[]>(() => {
    const patch = diffQ.data?.patch ?? "";
    if (!patch.trim()) return [];
    try {
      return parsePatchFiles(patch).flatMap((p) => p.files);
    } catch {
      return [];
    }
  }, [diffQ.data?.patch]);

  const tree = useMemo(() => buildTree(files), [files]);
  const orderedFiles = useMemo(() => flattenLeaves(tree), [tree]);

  const [collapsed, setCollapsed] = useState(loadSidebarCollapsed);
  // The file currently scrolled to the top of the viewport, highlighted in the
  // sidebar. Tracked separately from the heavy diff stack so scroll updates
  // only re-render the sidebar.
  const [activePath, setActivePath] = useState<string | null>(null);

  const scrollRef = useRef<HTMLDivElement | null>(null);
  const fileRefs = useRef<Map<string, HTMLElement>>(new Map());
  const rafRef = useRef(0);

  const toggleCollapsed = () => {
    setCollapsed((prev) => {
      const next = !prev;
      try {
        localStorage.setItem(SIDEBAR_COLLAPSED_KEY, next ? "1" : "0");
      } catch {
        // ignore
      }
      return next;
    });
  };

  // A single stable ref callback keyed off `data-path`, so the memoized diff
  // stack never sees a changing prop. Stale entries for removed files are never
  // read (we only iterate the current `orderedFiles`).
  const registerRef = useCallback((el: HTMLElement | null) => {
    const path = el?.dataset.path;
    if (el && path) fileRefs.current.set(path, el);
  }, []);

  // Mark the last file whose top has reached (or passed) the viewport top as
  // active — same behavior as GitHub's "Files changed" sidebar.
  const updateActive = useCallback(() => {
    const container = scrollRef.current;
    if (!container) return;
    const top = container.getBoundingClientRect().top;
    let current: string | null = null;
    for (const f of orderedFiles) {
      const el = fileRefs.current.get(f.name);
      if (!el) continue;
      if (el.getBoundingClientRect().top - top <= 8) current = f.name;
      else break;
    }
    setActivePath(current ?? orderedFiles[0]?.name ?? null);
  }, [orderedFiles]);

  const onScroll = () => {
    if (rafRef.current) return;
    rafRef.current = requestAnimationFrame(() => {
      rafRef.current = 0;
      updateActive();
    });
  };

  // Default to the first file and recompute once the diffs have laid out (file
  // heights aren't known until the first paint after a diff changes).
  useEffect(() => {
    setActivePath(orderedFiles[0]?.name ?? null);
    const id = requestAnimationFrame(() => updateActive());
    return () => cancelAnimationFrame(id);
  }, [orderedFiles, updateActive]);

  const scrollToFile = useCallback((path: string) => {
    const container = scrollRef.current;
    const el = fileRefs.current.get(path);
    if (!container || !el) return;
    const top =
      el.getBoundingClientRect().top - container.getBoundingClientRect().top + container.scrollTop;
    container.scrollTo({ top, behavior: "smooth" });
    setActivePath(path);
  }, []);

  if (diffQ.isLoading) {
    return <p className="p-4 text-sm text-fg-muted">Loading diff…</p>;
  }
  if (diffQ.error) {
    return <p className="p-4 text-sm text-danger">Couldn't load the diff.</p>;
  }
  if (orderedFiles.length === 0) {
    return <p className="p-4 text-sm text-fg-muted">No changes yet.</p>;
  }

  return (
    <div className="flex h-full min-h-0">
      {collapsed ? (
        <button
          type="button"
          onClick={toggleCollapsed}
          title="Show changed files"
          aria-label="Show changed files"
          aria-expanded={false}
          className="flex h-full w-6 shrink-0 items-center justify-center border-r border-border bg-bg text-fg-muted hover:bg-surface-2 hover:text-fg"
        >
          <span aria-hidden>▶</span>
        </button>
      ) : (
        <aside className="flex w-64 shrink-0 flex-col border-r border-border bg-surface text-sm">
          <div className="sticky top-0 z-10 flex items-center border-b border-border bg-surface px-3 py-2">
            <h2 className="text-xs font-semibold uppercase tracking-wide text-fg-muted">
              Changed files
            </h2>
            <span className="ml-2 text-xs text-fg-muted">{orderedFiles.length}</span>
            <button
              type="button"
              onClick={toggleCollapsed}
              title="Hide changed files"
              aria-label="Hide changed files"
              aria-expanded={true}
              className="ml-auto rounded px-1 text-fg-muted hover:bg-surface-2 hover:text-fg"
            >
              <span aria-hidden>◀</span>
            </button>
          </div>
          <div className="flex-1 overflow-y-auto [scrollbar-gutter:stable]">
            <TreeRows nodes={tree} depth={0} selected={activePath} onSelect={scrollToFile} />
          </div>
        </aside>
      )}
      <div
        ref={scrollRef}
        onScroll={onScroll}
        className="min-w-0 flex-1 overflow-auto [scrollbar-gutter:stable]"
      >
        <DiffStack
          files={orderedFiles}
          theme={theme}
          themeType={resolved}
          registerRef={registerRef}
        />
      </div>
    </div>
  );
}

// DiffStack renders every changed file's diff stacked vertically. It is
// memoized on (files, theme) so the active-file highlight — which changes on
// every scroll frame — never re-renders the (expensive) FileDiff list.
const DiffStack = memo(function DiffStack({
  files,
  theme,
  themeType,
  registerRef,
}: {
  files: FileDiffMetadata[];
  theme: string;
  themeType: "light" | "dark";
  registerRef: (el: HTMLElement | null) => void;
}) {
  return (
    <>
      {files.map((f) => (
        <div
          key={f.name}
          data-path={f.name}
          ref={registerRef}
          className="border-b border-border last:border-b-0"
        >
          {f.hunks.length === 0 ? (
            <div className="px-3 py-2">
              <div className="truncate font-mono text-xs text-fg" title={displayPath(f)}>
                {displayPath(f)}
              </div>
              <div className="text-sm text-fg-muted">
                No textual diff to display (binary file or metadata-only change).
              </div>
            </div>
          ) : (
            <FileDiff
              fileDiff={f}
              options={{
                diffStyle: "split",
                theme,
                themeType,
                stickyHeader: true,
                unsafeCSS: DIFF_UNSAFE_CSS,
              }}
              disableWorkerPool
            />
          )}
        </div>
      ))}
    </>
  );
});

function TreeRows({
  nodes,
  depth,
  selected,
  onSelect,
}: {
  nodes: TreeNode[];
  depth: number;
  selected: string | null;
  onSelect: (path: string) => void;
}) {
  return (
    <>
      {nodes.map((n) =>
        n.kind === "dir" ? (
          <DirRow key={n.path} dir={n} depth={depth} selected={selected} onSelect={onSelect} />
        ) : (
          <FileRow key={n.path} leaf={n} depth={depth} selected={selected} onSelect={onSelect} />
        ),
      )}
    </>
  );
}

function DirRow({
  dir,
  depth,
  selected,
  onSelect,
}: {
  dir: Dir;
  depth: number;
  selected: string | null;
  onSelect: (path: string) => void;
}) {
  const [open, setOpen] = useState(true);
  return (
    <div>
      <button
        type="button"
        onClick={() => setOpen((o) => !o)}
        style={{ paddingLeft: depth * 12 + 8 }}
        className="flex w-full items-center gap-1 py-1 pr-2 text-left text-fg-muted hover:bg-bg"
        title={dir.path}
      >
        <span aria-hidden className="w-3 shrink-0 text-center text-[10px]">
          {open ? "▾" : "▸"}
        </span>
        <span className="min-w-0 flex-1 truncate">{dir.name}</span>
      </button>
      {open && (
        <TreeRows nodes={dir.children} depth={depth + 1} selected={selected} onSelect={onSelect} />
      )}
    </div>
  );
}

function FileRow({
  leaf,
  depth,
  selected,
  onSelect,
}: {
  leaf: Leaf;
  depth: number;
  selected: string | null;
  onSelect: (path: string) => void;
}) {
  const { adds, dels } = fileStats(leaf.file);
  const marker = changeMarker(leaf.file.type);
  const active = selected === leaf.path;
  return (
    <button
      type="button"
      onClick={() => onSelect(leaf.path)}
      title={leaf.file.prevName ? `${leaf.file.prevName} → ${leaf.path}` : leaf.path}
      style={{ paddingLeft: depth * 12 + 8 }}
      className={`flex w-full items-center gap-2 py-1 pr-2 text-left hover:bg-bg ${
        active ? "bg-bg font-medium" : ""
      }`}
    >
      <span className={`w-3 shrink-0 text-center text-[10px] font-bold ${marker.cls}`}>
        {marker.label}
      </span>
      <span className="min-w-0 flex-1 truncate">{leaf.name}</span>
      {adds > 0 && <span className="shrink-0 text-[10px] text-[#3fb950]">+{adds}</span>}
      {dels > 0 && <span className="shrink-0 text-[10px] text-danger">−{dels}</span>}
    </button>
  );
}
