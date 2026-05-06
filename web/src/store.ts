import { useCallback, useSyncExternalStore } from "react";
import { api } from "@/api/client";
import type {
  Board,
  Column,
  MergeConfig,
  Session,
  SyncConfig,
  Ticket,
} from "@/api/client";

// Per-id subscriptions: a render that reads `useEntity(store, id)` only
// re-runs when that one id's value changes. By construction, updating ticket 5
// notifies only ticket-5 subscribers — no cascade through Board/Column.
export class EntityStore<T> {
  private values = new Map<number, T>();
  private idListeners = new Map<number, Set<() => void>>();

  get = (id: number): T | undefined => this.values.get(id);

  set = (id: number, value: T): void => {
    if (this.values.get(id) === value) return;
    this.values.set(id, value);
    this.idListeners.get(id)?.forEach((l) => l());
  };

  delete = (id: number): void => {
    if (!this.values.has(id)) return;
    this.values.delete(id);
    this.idListeners.get(id)?.forEach((l) => l());
  };

  subscribe = (id: number, cb: () => void): (() => void) => {
    let set = this.idListeners.get(id);
    if (!set) {
      set = new Set();
      this.idListeners.set(id, set);
    }
    set.add(cb);
    return () => {
      set!.delete(cb);
    };
  };

  // Reconciliation hook: walk every (id, value) pair so callers can drop
  // entries that no longer belong (e.g. tickets removed from a board).
  forEach = (cb: (value: T, id: number) => void): void => {
    this.values.forEach(cb);
  };
}

// A single value with subscribe/select. `useScalarSelector` is the hot path:
// each subscriber renders only when its derived selection changes (`Object.is`).
export class ScalarStore<T> {
  private value: T;
  private listeners = new Set<() => void>();

  constructor(initial: T) {
    this.value = initial;
  }

  get = (): T => this.value;

  set = (next: T | ((prev: T) => T)): void => {
    const v =
      typeof next === "function" ? (next as (p: T) => T)(this.value) : next;
    if (Object.is(v, this.value)) return;
    this.value = v;
    this.listeners.forEach((l) => l());
  };

  subscribe = (cb: () => void): (() => void) => {
    this.listeners.add(cb);
    return () => {
      this.listeners.delete(cb);
    };
  };
}

export function useEntity<T>(
  store: EntityStore<T>,
  id: number,
): T | undefined {
  const subscribe = useCallback(
    (cb: () => void) => store.subscribe(id, cb),
    [store, id],
  );
  const getSnapshot = useCallback(() => store.get(id), [store, id]);
  return useSyncExternalStore(subscribe, getSnapshot);
}

export function useScalarStore<T>(store: ScalarStore<T>): T {
  return useSyncExternalStore(store.subscribe, store.get);
}

export function useScalarSelector<T, S>(
  store: ScalarStore<T>,
  select: (v: T) => S,
): S {
  const getSnapshot = useCallback(() => select(store.get()), [store, select]);
  return useSyncExternalStore(store.subscribe, getSnapshot);
}

// Singletons. Tickets and sessions live here; the boardStructure query in
// React Query holds only ids and column/session shape so structural changes
// (column moves, archives, creates) refetch independently of content edits.
export const ticketStore = new EntityStore<Ticket>();
export const sessionStore = new EntityStore<Session>();
export const activeTicketStore = new ScalarStore<number | null>(null);

export function useTicket(id: number): Ticket | undefined {
  return useEntity(ticketStore, id);
}

export function useSession(id: number | null | undefined): Session | undefined {
  return useEntity(sessionStore, id ?? -1);
}

export function useActiveTicketId(): number | null {
  return useScalarStore(activeTicketStore);
}

export function useIsActiveTicket(id: number): boolean {
  return useScalarSelector(activeTicketStore, (v) => v === id);
}

// BoardStructure is the *shape* of a board: which columns exist and which
// ticket-ids live in each, in render order. The actual ticket and session
// content lives in `ticketStore` / `sessionStore` so per-id subscribers
// re-render in isolation. Keep this object purely structural — anything that
// changes per content edit (title, status, body, …) must NOT live here.
export type BoardStructure = {
  board: Board;
  columns: Column[];
  ticketIdsByColumn: Record<number, number[]>;
  sessionIdByTicket: Record<number, number>;
  merge_config: MergeConfig;
  sync_config: SyncConfig;
};

// fetchBoardStructure: fetch the server snapshot, fan content out into the
// entity stores, return the structural index. Reconciliation: any ticket
// previously in `ticketStore` for this board that isn't in the new snapshot
// (archived / deleted in another tab) is dropped, and so is its session.
export async function fetchBoardStructure(
  boardId: number,
): Promise<BoardStructure> {
  const data = await api.boardState(boardId);

  const newTicketIds = new Set(data.tickets.map((t) => t.id));
  for (const t of data.tickets) ticketStore.set(t.id, t);
  ticketStore.forEach((t, id) => {
    if (t.board_id === boardId && !newTicketIds.has(id)) ticketStore.delete(id);
  });

  const newSessionIds = new Set(data.sessions.map((s) => s.id));
  for (const s of data.sessions) sessionStore.set(s.id, s);
  sessionStore.forEach((s, id) => {
    if (newSessionIds.has(id)) return;
    // A session belongs to *this* board iff its ticket does. After we just
    // reconciled tickets above, lookup is authoritative.
    const t = ticketStore.get(s.ticket_id);
    if (!t || t.board_id === boardId) sessionStore.delete(id);
  });

  const ticketIdsByColumn: Record<number, number[]> = {};
  for (const c of data.columns) ticketIdsByColumn[c.id] = [];
  const sorted = [...data.tickets].sort((a, b) => a.position - b.position);
  for (const t of sorted) {
    const arr = ticketIdsByColumn[t.column_id];
    if (arr) arr.push(t.id);
  }

  const sessionIdByTicket: Record<number, number> = {};
  for (const s of data.sessions) sessionIdByTicket[s.ticket_id] = s.id;

  return {
    board: data.board,
    columns: data.columns,
    ticketIdsByColumn,
    sessionIdByTicket,
    merge_config: data.merge_config,
    sync_config: data.sync_config,
  };
}
