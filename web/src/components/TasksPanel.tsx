import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useRef, useState } from "react";
import { api, ApiError, Session } from "../api/client";
import { useToast } from "../toast";
import { Button } from "./Button";

function errorMessage(err: unknown): string {
  if (err instanceof ApiError) return err.message;
  if (err instanceof Error) return err.message;
  return String(err);
}

export function TasksPanel({ session }: { session: Session; boardId: number }) {
  const qc = useQueryClient();
  const toast = useToast();
  const tasksQ = useQuery({ queryKey: ["tasks", session.id], queryFn: () => api.discoverTasks(session.id) });
  const runsQ = useQuery({
    queryKey: ["runs", session.id],
    queryFn: () => api.listTaskRuns(session.id),
    refetchInterval: 2000,
  });
  const portsQ = useQuery({
    queryKey: ["ports", session.id],
    queryFn: () => api.listPorts(session.id),
    refetchInterval: 2000,
  });

  const [openOutputId, setOpenOutputId] = useState<number | null>(null);

  const startMut = useMutation({
    mutationFn: (label: string) => api.startTaskRun(session.id, label),
    onSuccess: (run) => {
      setOpenOutputId(run.id);
      qc.invalidateQueries({ queryKey: ["runs", session.id] });
      qc.invalidateQueries({ queryKey: ["ports", session.id] });
    },
    onError: (err) => toast.push("error", errorMessage(err)),
  });
  const stopMut = useMutation({
    mutationFn: (id: number) => api.stopTaskRun(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["runs", session.id] }),
    onError: (err) => toast.push("error", errorMessage(err)),
  });

  if (session.status === "stopped") {
    return <p className="p-4 text-sm text-zinc-400">Start the session to discover tasks.</p>;
  }

  const tasks = tasksQ.data ?? [];
  const runs = runsQ.data ?? [];
  const ports = portsQ.data ?? [];
  const portByContainer = new Map(ports.map((p) => [p.container_port, p]));

  return (
    <div className="flex h-full flex-col gap-3 p-3 text-sm">
      <section>
        <h3 className="mb-2 text-xs uppercase tracking-wide text-zinc-400">Detected tasks</h3>
        {tasks.length === 0 && <p className="text-zinc-500">No .vscode/tasks.json or launch.json detected.</p>}
        <ul className="flex flex-col gap-1">
          {tasks.map((t) => {
            const port = t.has_port ? portByContainer.get(t.container_port!) : undefined;
            return (
              <li key={t.label} className="flex items-center justify-between rounded bg-zinc-900 px-2 py-1">
                <div>
                  <div className="font-medium">{t.label}</div>
                  <div className="text-xs text-zinc-500">{t.command} {t.args?.join(" ")}</div>
                </div>
                <div className="flex items-center gap-2">
                  {t.has_port && (
                    <span className="text-xs text-zinc-400">
                      :{t.container_port}
                      {port && (
                        <a className="ml-1 text-red-400" href={`http://localhost:${port.host_port}`} target="_blank" rel="noreferrer">
                          → :{port.host_port}
                        </a>
                      )}
                    </span>
                  )}
                  <Button
                    variant="primary"
                    size="sm"
                    onClick={() => startMut.mutate(t.label)}
                    pending={startMut.isPending && startMut.variables === t.label}
                    idleLabel="run"
                    pendingLabel="starting…"
                  />
                </div>
              </li>
            );
          })}
        </ul>
      </section>
      <section className="min-h-0 flex-1 overflow-y-auto">
        <h3 className="mb-2 text-xs uppercase tracking-wide text-zinc-400">Runs</h3>
        <ul className="flex flex-col gap-1">
          {runs.map((r) => (
            <li key={r.id} className="rounded bg-zinc-900 p-2">
              <div className="flex items-center justify-between">
                <div>
                  <div className="font-medium">{r.task_label}</div>
                  <div className="text-xs text-zinc-500">{r.status}{r.exit_code != null ? ` (exit ${r.exit_code})` : ""}</div>
                </div>
                <div className="flex gap-2">
                  <Button variant="ghost" size="sm" onClick={() => setOpenOutputId(openOutputId === r.id ? null : r.id)}>
                    {openOutputId === r.id ? "hide output" : "output"}
                  </Button>
                  {r.status === "running" && (
                    <Button
                      variant="neutral"
                      size="sm"
                      onClick={() => stopMut.mutate(r.id)}
                      pending={stopMut.isPending && stopMut.variables === r.id}
                      idleLabel="stop"
                      pendingLabel="stopping…"
                    />
                  )}
                </div>
              </div>
              {openOutputId === r.id && <TaskOutput runId={r.id} />}
            </li>
          ))}
        </ul>
      </section>
    </div>
  );
}

function TaskOutput({ runId }: { runId: number }) {
  const [lines, setLines] = useState<string[]>([]);
  const [copied, setCopied] = useState(false);
  const ref = useRef<HTMLPreElement>(null);

  useEffect(() => {
    const es = new EventSource(`/api/task-runs/${runId}/output`);
    es.onmessage = (e) => setLines((prev) => [...prev.slice(-500), e.data]);
    es.addEventListener("end", () => es.close());
    return () => es.close();
  }, [runId]);

  useEffect(() => {
    ref.current?.scrollTo({ top: ref.current.scrollHeight });
  }, [lines]);

  const onCopy = () => {
    navigator.clipboard.writeText(lines.join("\n"));
    setCopied(true);
    setTimeout(() => setCopied(false), 1500);
  };

  return (
    <div className="mt-2">
      <div className="mb-1 flex justify-end">
        <Button variant="ghost" size="sm" onClick={onCopy}>
          {copied ? "copied" : "copy"}
        </Button>
      </div>
      <pre ref={ref} className="max-h-64 overflow-y-auto rounded bg-black p-2 text-xs leading-tight text-zinc-200">
        {lines.join("\n")}
      </pre>
    </div>
  );
}
