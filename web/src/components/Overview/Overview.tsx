import { useCallback, useRef, useState } from "react";
import { BoardTree } from "./BoardTree";
import { PanelCanvas, type PanelCanvasHandle } from "./PanelCanvas";

const SIDEBAR_WIDTH_KEY = "overview.sidebar.width";
const DEFAULT_SIDEBAR = 280;
const MIN_SIDEBAR = 200;
const MAX_SIDEBAR = 600;

function loadSidebarWidth(): number {
  try {
    const raw = localStorage.getItem(SIDEBAR_WIDTH_KEY);
    const n = raw ? Number(raw) : NaN;
    if (!Number.isFinite(n)) return DEFAULT_SIDEBAR;
    return Math.min(MAX_SIDEBAR, Math.max(MIN_SIDEBAR, n));
  } catch {
    return DEFAULT_SIDEBAR;
  }
}

export function Overview() {
  const handleRef = useRef<PanelCanvasHandle | null>(null);
  const registerHandle = useCallback((h: PanelCanvasHandle | null) => {
    handleRef.current = h;
  }, []);
  const onOpenTicket = useCallback((boardId: number, ticketId: number) => {
    handleRef.current?.open(boardId, ticketId);
  }, []);

  const [sidebarWidth, setSidebarWidth] = useState(loadSidebarWidth);
  const [resizing, setResizing] = useState(false);

  const onResizeStart = (e: React.MouseEvent) => {
    e.preventDefault();
    setResizing(true);
    const onMove = (ev: MouseEvent) => {
      const w = Math.min(MAX_SIDEBAR, Math.max(MIN_SIDEBAR, ev.clientX));
      setSidebarWidth(w);
    };
    const onUp = () => {
      setResizing(false);
      window.removeEventListener("mousemove", onMove);
      window.removeEventListener("mouseup", onUp);
      try {
        localStorage.setItem(SIDEBAR_WIDTH_KEY, String(sidebarWidth));
      } catch {
        // ignore
      }
    };
    window.addEventListener("mousemove", onMove);
    window.addEventListener("mouseup", onUp);
  };

  return (
    <div className="flex h-full">
      <aside
        style={{ width: `${sidebarWidth}px`, flex: `0 0 ${sidebarWidth}px` }}
        className="flex h-full flex-col border-r border-border bg-bg"
      >
        <BoardTree onOpenTicket={onOpenTicket} />
      </aside>
      <div
        role="separator"
        aria-orientation="vertical"
        onMouseDown={onResizeStart}
        className={`w-1 cursor-col-resize hover:bg-accent-500/40 ${resizing ? "bg-accent-500/60" : ""}`}
      />
      <div className="min-w-0 flex-1">
        <PanelCanvas registerHandle={registerHandle} />
      </div>
    </div>
  );
}
