const ACTIVE_BOARD_KEY = "kanban.activeBoardId";

export function readActiveBoardId(): number | null {
  try {
    const raw = localStorage.getItem(ACTIVE_BOARD_KEY);
    if (!raw) return null;
    const id = Number(raw);
    return Number.isInteger(id) && id > 0 ? id : null;
  } catch {
    return null;
  }
}

export function writeActiveBoardId(id: number): void {
  try {
    localStorage.setItem(ACTIVE_BOARD_KEY, String(id));
  } catch {
    // ignore quota / disabled storage
  }
}
