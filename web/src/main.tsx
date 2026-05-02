import React from "react";
import ReactDOM from "react-dom/client";
import { MutationCache, QueryCache, QueryClient, QueryClientProvider } from "@tanstack/react-query";
import App from "@/App";
import { formatApiError } from "@/api/client";
import { ToastProvider, useToast } from "@/toast";
import "@/index.css";

function Root() {
  const { push } = useToast();
  const [client] = React.useState(
    () =>
      new QueryClient({
        defaultOptions: { queries: { staleTime: 5_000, refetchOnWindowFocus: false } },
        queryCache: new QueryCache({
          onError: (err) => push("error", formatApiError(err)),
        }),
        mutationCache: new MutationCache({
          onError: (err) => push("error", formatApiError(err)),
        }),
      }),
  );
  return (
    <QueryClientProvider client={client}>
      <App />
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
