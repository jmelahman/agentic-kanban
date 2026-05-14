import { useEffect, useRef } from "react";
import { init, Terminal, FitAddon, type ITheme } from "ghostty-web";
import { APPEARANCE_EVENT } from "@/hooks/useThemeMode";

const ghosttyReady = init();

function getTerminalTheme(): ITheme {
  const cs = getComputedStyle(document.documentElement);
  return {
    background: cs.getPropertyValue("--color-bg").trim(),
    foreground: cs.getPropertyValue("--color-fg").trim(),
  };
}

type Props = {
  sessionId: number;
  kind: "agent" | "shell";
  mountTarget: HTMLElement | null;
};

export function PtyTerminal({ sessionId, kind, mountTarget }: Props) {
  const wrapperRef = useRef<HTMLDivElement | null>(null);
  const hostRef = useRef<HTMLDivElement | null>(null);
  const fitRef = useRef<FitAddon | null>(null);
  const termRef = useRef<Terminal | null>(null);

  useEffect(() => {
    let cancelled = false;
    let cleanup: (() => void) | null = null;

    const wrapper = document.createElement("div");
    wrapper.style.position = "relative";
    wrapper.style.height = "100%";
    wrapper.style.width = "100%";

    const host = document.createElement("div");
    host.dataset.terminal = "true";
    host.style.height = "100%";
    host.style.width = "100%";
    wrapper.appendChild(host);

    wrapperRef.current = wrapper;
    hostRef.current = host;
    getOffscreenContainer().appendChild(wrapper);

    ghosttyReady.then(() => {
      if (cancelled) return;

      const term = new Terminal({
        fontFamily: "ui-monospace, SFMono-Regular, Menlo, monospace",
        fontSize: 13,
        theme: getTerminalTheme(),
      });
      const fit = new FitAddon();
      fitRef.current = fit;
      termRef.current = term;
      term.loadAddon(fit);
      term.open(host);
      fit.fit();
      // Bypass term.focus() — it calls host.focus() with no options, so the
      // browser scrolls the host into view. With the mobile soft keyboard
      // open the visual viewport is short, so that scroll lands the terminal
      // in the middle of the screen instead of pinned to the top of the pane.
      if (wrapper.parentElement !== getOffscreenContainer()) host.focus({ preventScroll: true });

      const proto = window.location.protocol === "https:" ? "wss:" : "ws:";
      const path = kind === "shell" ? "shell" : "pty";
      const ws = new WebSocket(
        `${proto}//${window.location.host}/ws/sessions/${sessionId}/${path}`,
      );
      ws.binaryType = "arraybuffer";

      const controls = createTerminalControls(term, () => {
        if (ws.readyState === WebSocket.OPEN) ws.send("\x0c");
      });
      wrapper.appendChild(controls.element);

      const sendResize = () => {
        ws.send(JSON.stringify({ type: "resize", cols: term.cols, rows: term.rows }));
      };

      ws.onopen = () => sendResize();
      ws.onmessage = (e) => {
        if (typeof e.data === "string") {
          term.write(e.data);
        } else {
          term.write(new Uint8Array(e.data));
        }
      };
      ws.onclose = () => term.write("\r\n[disconnected]\r\n");

      const dataDisp = term.onData((data) => {
        if (ws.readyState === WebSocket.OPEN) ws.send(data);
      });
      const resizeDisp = term.onResize(() => {
        if (ws.readyState === WebSocket.OPEN) sendResize();
      });

      let fitTimer: number | null = null;
      const scheduleFit = () => {
        if (fitTimer != null) clearTimeout(fitTimer);
        fitTimer = window.setTimeout(() => {
          fitTimer = null;
          fit.fit();
        }, 80);
      };
      const observer = new ResizeObserver(scheduleFit);
      observer.observe(host);

      const onAppearance = () => {
        term.options.theme = getTerminalTheme();
      };
      window.addEventListener(APPEARANCE_EVENT, onAppearance);

      cleanup = () => {
        if (fitTimer != null) clearTimeout(fitTimer);
        observer.disconnect();
        window.removeEventListener(APPEARANCE_EVENT, onAppearance);
        dataDisp.dispose();
        resizeDisp.dispose();
        // Drop handlers before close() — the close event fires async and
        // would otherwise call term.write() on the disposed terminal.
        ws.onopen = null;
        ws.onmessage = null;
        ws.onclose = null;
        ws.close();
        // Workaround for coder/ghostty-web#141: ghostty_terminal_free
        // corrupts the shared WASM heap if the terminal processed a
        // multi-codepoint grapheme cluster, breaking every later terminal on
        // the page. Null wasmTerm so dispose() skips that call. Remove once
        // the fix in coder/ghostty-web#142 ships.
        (term as unknown as { wasmTerm?: unknown }).wasmTerm = undefined;
        term.dispose();
        controls.dispose();
      };
    });

    return () => {
      cancelled = true;
      cleanup?.();
      wrapper.remove();
      wrapperRef.current = null;
      hostRef.current = null;
      fitRef.current = null;
      termRef.current = null;
    };
  }, [sessionId, kind]);

  useEffect(() => {
    const wrapper = wrapperRef.current;
    if (!wrapper) return;
    const target = mountTarget ?? getOffscreenContainer();
    if (wrapper.parentElement !== target) {
      target.appendChild(wrapper);
      fitRef.current?.fit();
      if (mountTarget) hostRef.current?.focus({ preventScroll: true });
    }
  }, [mountTarget]);

  return null;
}

function createTerminalControls(
  term: Terminal,
  onClear: () => void,
): {
  element: HTMLDivElement;
  dispose: () => void;
} {
  const container = document.createElement("div");
  container.className =
    "pointer-events-none absolute inset-y-0 right-2 z-10 hidden flex-col items-center justify-center gap-2 pointer-coarse:flex";

  const buttonClass =
    "pointer-events-auto flex h-10 w-10 items-center justify-center rounded-full border border-border bg-surface/80 text-fg shadow-md backdrop-blur-sm active:bg-surface-2";

  const timers = new Set<number>();
  const makeScrollButton = (label: string, glyph: string, direction: -1 | 1) => {
    const btn = document.createElement("button");
    btn.type = "button";
    btn.setAttribute("aria-label", label);
    btn.textContent = glyph;
    btn.className = buttonClass;
    // Don't steal focus from the terminal on tap.
    btn.addEventListener("mousedown", (e) => e.preventDefault());

    let timer: number | null = null;
    const stop = () => {
      if (timer != null) {
        clearInterval(timer);
        timers.delete(timer);
        timer = null;
      }
    };
    btn.addEventListener("pointerdown", (e) => {
      e.preventDefault();
      btn.setPointerCapture(e.pointerId);
      term.scrollLines(3 * direction);
      timer = window.setInterval(() => term.scrollLines(3 * direction), 100);
      timers.add(timer);
    });
    btn.addEventListener("pointerup", stop);
    btn.addEventListener("pointercancel", stop);
    btn.addEventListener("lostpointercapture", stop);
    return btn;
  };

  const clearBtn = document.createElement("button");
  clearBtn.type = "button";
  clearBtn.setAttribute("aria-label", "Clear screen");
  clearBtn.textContent = "⎚";
  clearBtn.className = buttonClass;
  clearBtn.addEventListener("mousedown", (e) => e.preventDefault());
  clearBtn.addEventListener("click", onClear);

  container.append(
    makeScrollButton("Scroll up", "▲", -1),
    clearBtn,
    makeScrollButton("Scroll down", "▼", 1),
  );

  return {
    element: container,
    dispose: () => {
      for (const t of timers) clearInterval(t);
      timers.clear();
    },
  };
}

let offscreenContainer: HTMLDivElement | null = null;
function getOffscreenContainer(): HTMLDivElement {
  if (offscreenContainer?.isConnected) return offscreenContainer;
  const div = document.createElement("div");
  div.style.position = "absolute";
  div.style.left = "-99999px";
  div.style.top = "-99999px";
  div.style.width = "640px";
  div.style.height = "400px";
  div.style.pointerEvents = "none";
  document.body.appendChild(div);
  offscreenContainer = div;
  return div;
}
