# Observability

The kanban backend exposes a Prometheus exposition endpoint at `GET /metrics`
covering four signals:

- **HTTP server** — `kanban_http_requests_total{route,method,status}` and
  `kanban_http_request_duration_seconds{route,method}`. `route` is the Go
  1.22+ `ServeMux` pattern, so dynamic IDs collapse into a single label.
- **GitHub REST API** — `kanban_github_api_requests_total{endpoint,method,status}`,
  `kanban_github_api_request_duration_seconds{endpoint,method}`, and three
  gauges sourced from `X-RateLimit-*` response headers:
  `kanban_github_rate_limit_remaining{resource}`,
  `kanban_github_rate_limit_limit{resource}`, and
  `kanban_github_rate_limit_reset_timestamp_seconds{resource}`. `resource`
  mirrors GitHub's bucketing (`core`, `search`, `graphql`, ...).
- **SQLite queries** — `kanban_db_query_duration_seconds{op,target}` and
  `kanban_db_query_errors_total{op,target}`. `op` is the SQL verb
  (`select`, `insert`, `update`, `delete`, `tx`, `pragma`, `ddl`, ...) and
  `target` is the first identifier after the verb (table name for DML,
  pragma name for `PRAGMA`, etc.).
- **Go runtime / process** — standard collectors from `prometheus/client_golang`.

## Bundled Prometheus service

`compose.yaml` includes a `prometheus` service on `kanban-net` that scrapes
`kanban:7474/metrics` every 15s. The TSDB persists in the `prometheus-data`
named volume so a `docker compose restart` keeps history.

The Prometheus UI is **not** published as a host port. Instead, the kanban
backend reverse-proxies it under `/prometheus/`, so the UI is reachable at:

```
http://localhost:7474/prometheus/
```

The proxy target is `$KANBAN_PROMETHEUS_URL` (set to `http://prometheus:9090`
in `compose.yaml`). When the env var is unset (e.g. `wgo run . serve` in
dev) `/prometheus/` returns `503 Service Unavailable` — the `/metrics`
endpoint still works on its own, so a one-off `curl localhost:7474/metrics`
works without Prometheus running.

## Diagnosing GitHub rate-limit issues

Both the PR poller (`internal/github`) and the build-cop poller
(`internal/buildcop`) flow through the instrumented transport, so a query
like:

```promql
sum by (endpoint) (rate(kanban_github_api_requests_total[5m]))
```

shows which endpoint is burning budget, and:

```promql
kanban_github_rate_limit_remaining{resource="core"}
```

surfaces the absolute remaining budget against the per-hour cap.

If the poll cadence is too aggressive for the install:

- **Build-cop** (the heavier of the two pollers) accepts
  `[buildcop].interval` in `.kanban.toml` (default `2m`). The jobs-per-run
  endpoint is the main offender on this poller — completed workflow runs
  are immutable, so they're cached in memory after the first fetch and
  re-used until they age out of the rolling window.
- **PR poller** ticks every 30s and is bounded to 2 pages of 100 PRs per
  repo per tick.

## Customising the scrape config

`deploy/prometheus.yml` defines a single static job that scrapes the kanban
backend. Mount additional rules or extend the file when adding new
exporters (e.g. node_exporter, cadvisor). The file is mounted read-only
into the container; edit on disk and `docker compose restart prometheus`
to reload.
