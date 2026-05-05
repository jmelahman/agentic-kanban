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
    id: "ticket.prev",
    label: "Previous ticket",
    group: "Navigation",
    defaultBinding: { ctrl: true, alt: true, shift: false, meta: false, key: "ArrowUp" },
  },
  {
    id: "ticket.next",
    label: "Next ticket",
    group: "Navigation",
    defaultBinding: { ctrl: true, alt: true, shift: false, meta: false, key: "ArrowDown" },
  },
  {
    id: "column.prev",
    label: "Previous column",
    group: "Navigation",
    defaultBinding: { ctrl: true, alt: true, shift: false, meta: false, key: "ArrowLeft" },
  },
  {
    id: "column.next",
    label: "Next column",
    group: "Navigation",
    defaultBinding: { ctrl: true, alt: true, shift: false, meta: false, key: "ArrowRight" },
  },
];

export const ACTIONS_BY_ID: Record<string, Action> = Object.fromEntries(
  ACTIONS.map((a) => [a.id, a]),
);
