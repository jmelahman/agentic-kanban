import type { Action } from "./types";

export const ACTIONS: Action[] = [
  {
    id: "tab.next",
    label: "Next tab",
    group: "Session",
    defaultBinding: { ctrl: true, alt: true, shift: false, meta: false, key: "PageDown" },
  },
  {
    id: "tab.prev",
    label: "Previous tab",
    group: "Session",
    defaultBinding: { ctrl: true, alt: true, shift: false, meta: false, key: "PageUp" },
  },
  {
    id: "board.next",
    label: "Next board",
    group: "Navigation",
    defaultBinding: { ctrl: true, alt: true, shift: false, meta: false, key: "ArrowRight" },
  },
  {
    id: "board.prev",
    label: "Previous board",
    group: "Navigation",
    defaultBinding: { ctrl: true, alt: true, shift: false, meta: false, key: "ArrowLeft" },
  },
];

export const ACTIONS_BY_ID: Record<string, Action> = Object.fromEntries(
  ACTIONS.map((a) => [a.id, a]),
);
