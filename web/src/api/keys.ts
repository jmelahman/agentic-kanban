export const queryKeys = {
  boards: ["boards"] as const,
  board: (boardId: number) => ["board", boardId] as const,
  archived: (boardId: number) => ["archived", boardId] as const,
  tasks: (sessionId: number) => ["tasks", sessionId] as const,
  runs: (sessionId: number) => ["runs", sessionId] as const,
  ports: (sessionId: number) => ["ports", sessionId] as const,
  settings: ["settings"] as const,
  harnesses: ["harnesses"] as const,
  version: ["version"] as const,
};
