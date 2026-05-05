import { useEffect, useState } from "react";
import { APPEARANCE_EVENT } from "./useThemeMode";

export type Contrast = "normal" | "high";

const STORAGE_KEY = "kanban.contrast";
const EVENT = "kanban:contrast";

function read(): Contrast {
  try {
    return localStorage.getItem(STORAGE_KEY) === "high" ? "high" : "normal";
  } catch {
    return "normal";
  }
}

function apply(value: Contrast): void {
  if (value === "high") document.documentElement.dataset.contrast = "high";
  else delete document.documentElement.dataset.contrast;
}

export function setContrast(value: Contrast): void {
  try {
    localStorage.setItem(STORAGE_KEY, value);
  } catch {
    // ignore
  }
  apply(value);
  window.dispatchEvent(new CustomEvent<Contrast>(EVENT, { detail: value }));
  window.dispatchEvent(new Event(APPEARANCE_EVENT));
}

export function useContrast(): Contrast {
  const [contrast, setState] = useState<Contrast>(() => read());
  useEffect(() => apply(contrast), [contrast]);
  useEffect(() => {
    const onCustom = (e: Event) => setState((e as CustomEvent<Contrast>).detail);
    const onStorage = (e: StorageEvent) => {
      if (e.key === STORAGE_KEY) setState(read());
    };
    window.addEventListener(EVENT, onCustom);
    window.addEventListener("storage", onStorage);
    return () => {
      window.removeEventListener(EVENT, onCustom);
      window.removeEventListener("storage", onStorage);
    };
  }, []);
  return contrast;
}
