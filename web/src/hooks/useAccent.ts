import { useEffect, useState } from "react";
import { APPEARANCE_EVENT } from "./useThemeMode";

export const ACCENTS = [
  "red",
  "orange",
  "amber",
  "green",
  "teal",
  "blue",
  "indigo",
  "purple",
  "pink",
] as const;

export type Accent = (typeof ACCENTS)[number];

const STORAGE_KEY = "kanban.accent";
const EVENT = "kanban:accent";

function read(): Accent {
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (raw && (ACCENTS as readonly string[]).includes(raw)) return raw as Accent;
    return "red";
  } catch {
    return "red";
  }
}

function apply(value: Accent): void {
  document.documentElement.dataset.accent = value;
}

export function setAccent(value: Accent): void {
  try {
    localStorage.setItem(STORAGE_KEY, value);
  } catch {
    // ignore
  }
  apply(value);
  window.dispatchEvent(new CustomEvent<Accent>(EVENT, { detail: value }));
  window.dispatchEvent(new Event(APPEARANCE_EVENT));
}

export function useAccent(): Accent {
  const [accent, setState] = useState<Accent>(() => read());
  useEffect(() => apply(accent), [accent]);
  useEffect(() => {
    const onCustom = (e: Event) => setState((e as CustomEvent<Accent>).detail);
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
  return accent;
}
