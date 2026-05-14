import { useQuery } from "@tanstack/react-query";
import { type HTMLAttributes, useEffect, useMemo, useState } from "react";
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import { api, type Session } from "@/api/client";
import { queryKeys } from "@/api/keys";

export function PlansPanel({ session }: { session: Session }) {
  const plansQ = useQuery({
    queryKey: queryKeys.sessionPlans(session.id),
    queryFn: () => api.listSessionPlans(session.id),
    refetchInterval: 5000,
  });

  const sorted = useMemo(() => {
    const list = plansQ.data ?? [];
    return [...list].sort((a, b) => (b.mod_time > a.mod_time ? 1 : -1));
  }, [plansQ.data]);

  const [selected, setSelected] = useState<string | null>(null);

  // Re-pin the default selection whenever the list changes — if the
  // previously selected file disappears, fall back to the newest one.
  useEffect(() => {
    if (sorted.length === 0) {
      setSelected(null);
      return;
    }
    if (selected != null && sorted.some((p) => p.name === selected)) return;
    setSelected(sorted[0].name);
  }, [sorted, selected]);

  if (plansQ.isLoading) {
    return <p className="p-4 text-sm text-fg-muted">Loading plans…</p>;
  }
  if (sorted.length === 0) {
    return <p className="p-4 text-sm text-fg-muted">No plan files found.</p>;
  }

  return (
    <div className="flex h-full min-h-0">
      <aside className="flex w-56 shrink-0 flex-col border-r border-border bg-surface text-sm">
        <ul className="flex-1 overflow-y-auto [scrollbar-gutter:stable]">
          {sorted.map((p) => (
            <li key={p.name}>
              <button
                type="button"
                onClick={() => setSelected(p.name)}
                className={`block w-full truncate px-3 py-1.5 text-left hover:bg-bg ${
                  selected === p.name ? "bg-bg font-medium" : ""
                }`}
                title={p.name}
              >
                {p.name.replace(/\.md$/i, "")}
              </button>
            </li>
          ))}
        </ul>
      </aside>
      <div className="min-w-0 flex-1 overflow-y-auto [scrollbar-gutter:stable]">
        {selected && <PlanContent sessionId={session.id} name={selected} />}
      </div>
    </div>
  );
}

function PlanContent({ sessionId, name }: { sessionId: number; name: string }) {
  const q = useQuery({
    queryKey: queryKeys.sessionPlan(sessionId, name),
    queryFn: () => api.getSessionPlan(sessionId, name),
  });

  if (q.isLoading) {
    return <p className="p-4 text-sm text-fg-muted">Loading…</p>;
  }
  if (q.error) {
    return <p className="p-4 text-sm text-danger">Couldn't load {name}</p>;
  }

  return (
    <article className="p-4 text-sm leading-relaxed text-fg">
      <ReactMarkdown remarkPlugins={[remarkGfm]} components={MARKDOWN_COMPONENTS}>
        {q.data ?? ""}
      </ReactMarkdown>
    </article>
  );
}

// Inline Tailwind on each markdown element keeps us off the
// `@tailwindcss/typography` plugin (not installed here) while still
// producing usable hierarchy.
const MARKDOWN_COMPONENTS = {
  h1: ({ ...props }) => <h1 className="mb-3 mt-4 text-xl font-semibold" {...props} />,
  h2: ({ ...props }) => <h2 className="mb-2 mt-4 text-lg font-semibold" {...props} />,
  h3: ({ ...props }) => <h3 className="mb-2 mt-3 text-base font-semibold" {...props} />,
  h4: ({ ...props }) => <h4 className="mb-1 mt-2 text-sm font-semibold" {...props} />,
  p: ({ ...props }) => <p className="my-2" {...props} />,
  ul: ({ ...props }) => <ul className="my-2 list-disc pl-6" {...props} />,
  ol: ({ ...props }) => <ol className="my-2 list-decimal pl-6" {...props} />,
  li: ({ ...props }) => <li className="my-0.5" {...props} />,
  a: ({ ...props }) => (
    <a className="text-accent-500 hover:underline" target="_blank" rel="noreferrer" {...props} />
  ),
  blockquote: ({ ...props }) => (
    <blockquote className="my-2 border-l-2 border-border pl-3 italic text-fg-muted" {...props} />
  ),
  code: ({ className, children, ...props }: HTMLAttributes<HTMLElement>) => {
    const isBlock = typeof className === "string" && className.startsWith("language-");
    if (isBlock) {
      return (
        <code className={`${className} block`} {...props}>
          {children}
        </code>
      );
    }
    return (
      <code className="rounded bg-surface px-1 py-0.5 font-mono text-xs" {...props}>
        {children}
      </code>
    );
  },
  pre: ({ ...props }) => (
    <pre
      className="my-2 overflow-x-auto rounded bg-surface p-3 font-mono text-xs leading-snug"
      {...props}
    />
  ),
  table: ({ ...props }) => (
    <div className="my-2 overflow-x-auto">
      <table className="w-full border-collapse text-left text-xs" {...props} />
    </div>
  ),
  th: ({ ...props }) => (
    <th className="border-b border-border px-2 py-1 font-semibold" {...props} />
  ),
  td: ({ ...props }) => <td className="border-b border-border px-2 py-1" {...props} />,
  hr: ({ ...props }) => <hr className="my-4 border-border" {...props} />,
};
