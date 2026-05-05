import React from "react";
import ReactDOM from "react-dom/client";
import { MutationCache, QueryCache, QueryClient, QueryClientProvider } from "@tanstack/react-query";
import App from "@/App";
import { ApiError, formatApiError, postError } from "@/api/client";
import { ErrorBoundary } from "@/components/ErrorBoundary";
import { ToastProvider, useToast } from "@/toast";
import "@/index.css";

// reportRuntimeError forwards unexpected errors to the backend. 4xx ApiErrors
// are user-visible business errors (board renamed in another tab, validation
// failures, …), not bugs — those toast but don't get reported.
function reportRuntimeError(err: unknown, source: string) {
  if (err instanceof ApiError && err.status < 500) return;
  void postError({
    message: formatApiError(err),
    stack: err instanceof Error ? err.stack : undefined,
    source,
  });
}

function Root() {
  const { push } = useToast();
  const [client] = React.useState(
    () =>
      new QueryClient({
        defaultOptions: { queries: { staleTime: 5_000, refetchOnWindowFocus: false } },
        queryCache: new QueryCache({
          onError: (err) => {
            push("error", formatApiError(err));
            reportRuntimeError(err, "react-query");
          },
        }),
        mutationCache: new MutationCache({
          onError: (err) => {
            push("error", formatApiError(err));
            reportRuntimeError(err, "react-query");
          },
        }),
      }),
  );
  return (
    <QueryClientProvider client={client}>
      <ErrorBoundary>
        <App />
      </ErrorBoundary>
    </QueryClientProvider>
  );
}

ReactDOM.createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <ToastProvider>
      <Root />
    </ToastProvider>
  </React.StrictMode>,
);
