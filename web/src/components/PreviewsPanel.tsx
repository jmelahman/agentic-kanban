import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, type Preview, type Session } from "@/api/client";
import { queryKeys } from "@/api/keys";
import { Button } from "./Button";

// Preview deployments for this session's branch: each deploy is a live,
// content-addressed build of a commit served at its own subdomain by the
// embedded local-preview orchestrator.

const statusColor: Record<Preview["status"], string> = {
  queued: "text-fg-muted",
  building: "text-accent-500",
  ready: "text-emerald-400",
  failed: "text-danger",
  evicted: "text-fg-muted line-through",
};

export function PreviewsPanel({ session }: { session: Session }) {
  const qc = useQueryClient();
  const previewsQ = useQuery({
    queryKey: queryKeys.previews(session.id),
    queryFn: () => api.listPreviews(session.id),
    refetchInterval: (query) =>
      query.state.data?.some((p) => p.status === "queued" || p.status === "building") ? 1500 : 5000,
  });

  const deployMut = useMutation({
    mutationFn: () => api.createPreview(session.id),
    onSuccess: () => qc.invalidateQueries({ queryKey: queryKeys.previews(session.id) }),
  });

  const previews = previewsQ.data ?? [];

  return (
    <div className="flex h-full flex-col gap-3 overflow-y-auto p-3 text-sm [scrollbar-gutter:stable]">
      <section>
        <div className="mb-2 flex items-center justify-between">
          <h3 className="text-xs uppercase tracking-wide text-fg-muted">
            Previews · {session.branch_name}
          </h3>
          <Button
            variant="primary"
            size="sm"
            onClick={() => deployMut.mutate()}
            pending={deployMut.isPending}
            idleLabel="deploy tip"
            pendingLabel="deploying…"
          />
        </div>
        {deployMut.isError && <p className="mb-2 text-xs text-danger">{String(deployMut.error)}</p>}
        {previewsQ.isError && (
          <p className="text-fg-muted">Previews unavailable: {String(previewsQ.error)}</p>
        )}
        {!previewsQ.isError && previews.length === 0 && (
          <p className="text-fg-muted">
            No previews yet. "deploy tip" builds this branch's latest commit and serves it at its
            own subdomain. The repo needs a preview.toml at its root.
          </p>
        )}
        <ul className="flex flex-col gap-1">
          {previews.map((p) => (
            <li
              key={p.id}
              className="flex items-center justify-between rounded bg-surface px-2 py-1"
            >
              <div>
                <div className="font-mono text-xs font-medium">{p.short_sha}</div>
                <div className="text-xs">
                  <span className={statusColor[p.status]}>{p.status}</span>
                  {p.process ? <span className="text-fg-muted"> · backend {p.process}</span> : null}
                  {p.status === "failed" && p.error ? (
                    <span className="text-danger"> — {p.error}</span>
                  ) : null}
                </div>
              </div>
              <div className="flex items-center gap-2">
                <a
                  className="text-xs text-fg-muted hover:text-fg"
                  href={`/api/previews/${p.id}/logs`}
                  target="_blank"
                  rel="noreferrer"
                >
                  logs
                </a>
                {p.status === "ready" && p.preview_url && (
                  <a
                    className="text-accent-500"
                    href={p.preview_url}
                    target="_blank"
                    rel="noreferrer"
                  >
                    open ↗
                  </a>
                )}
              </div>
            </li>
          ))}
        </ul>
      </section>
    </div>
  );
}
