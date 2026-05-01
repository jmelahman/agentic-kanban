import { useEffect, useRef } from "react";
import { Terminal } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";

type Props = {
  sessionId: number;
  mountTarget: HTMLElement | null;
};

export function PtyTerminal({ sessionId, mountTarget }: Props) {
  const hostRef = useRef<HTMLDivElement | null>(null);
  const fitRef = useRef<FitAddon | null>(null);

  useEffect(() => {
    const host = document.createElement("div");
    host.style.height = "100%";
    host.style.width = "100%";
    hostRef.current = host;
    getOffscreenContainer().appendChild(host);

    const term = new Terminal({
      convertEol: true,
      fontFamily: "ui-monospace, SFMono-Regular, Menlo, monospace",
      fontSize: 13,
      theme: { background: "#09090b" },
    });
    const fit = new FitAddon();
    fitRef.current = fit;
    term.loadAddon(fit);
    term.open(host);
    fit.fit();

    const proto = window.location.protocol === "https:" ? "wss:" : "ws:";
    const ws = new WebSocket(`${proto}//${window.location.host}/ws/sessions/${sessionId}/pty`);
    ws.binaryType = "arraybuffer";

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

    const observer = new ResizeObserver(() => fit.fit());
    observer.observe(host);

    return () => {
      observer.disconnect();
      dataDisp.dispose();
      resizeDisp.dispose();
      ws.close();
      term.dispose();
      host.remove();
      hostRef.current = null;
      fitRef.current = null;
    };
  }, [sessionId]);

  useEffect(() => {
    const host = hostRef.current;
    if (!host) return;
    const target = mountTarget ?? getOffscreenContainer();
    if (host.parentElement !== target) {
      target.appendChild(host);
      fitRef.current?.fit();
    }
  }, [mountTarget]);

  return null;
}

let offscreenContainer: HTMLDivElement | null = null;
function getOffscreenContainer(): HTMLDivElement {
  if (offscreenContainer && offscreenContainer.isConnected) return offscreenContainer;
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
