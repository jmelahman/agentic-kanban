import { useEffect, useState } from "react";

export type TerminalOrientation = "vertical" | "horizontal";

const STORAGE_KEY = "sessionPane.orientation";
const EVENT = "kanban:terminal-orientation";

function read(): TerminalOrientation {
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    return raw === "horizontal" ? "horizontal" : "vertical";
  } catch {
    return "vertical";
  }
}

export function setTerminalOrientation(value: TerminalOrientation): void {
  try {
    localStorage.setItem(STORAGE_KEY, value);
  } catch {
    // ignore quota / disabled storage
  }
  window.dispatchEvent(new CustomEvent<TerminalOrientation>(EVENT, { detail: value }));
}

export function useTerminalOrientation(): TerminalOrientation {
  const [orientation, setOrientation] = useState<TerminalOrientation>(() => read());
  useEffect(() => {
    const onCustom = (e: Event) => setOrientation((e as CustomEvent<TerminalOrientation>).detail);
    const onStorage = (e: StorageEvent) => {
      if (e.key === STORAGE_KEY) setOrientation(read());
    };
    window.addEventListener(EVENT, onCustom);
    window.addEventListener("storage", onStorage);
    return () => {
      window.removeEventListener(EVENT, onCustom);
      window.removeEventListener("storage", onStorage);
    };
  }, []);
  return orientation;
}
