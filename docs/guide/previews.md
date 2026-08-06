# Previews

Every ticket branch can be deployed as a live preview: a real build of the
branch's latest commit, served at its own subdomain like

```
http://a1b2c3d.my-board.preview.localhost:7474/
```

Previews are powered by an embedded
[local-preview](https://jmelahman.github.io/local-preview/) orchestrator:
frontend and backend are content-addressed separately (commits that don't
touch a side reuse the existing artifact and even the running backend
process), backend processes start on demand, and backend state follows git
lineage — a new backend version forks its data from the nearest deployed
ancestor commit, so previews on one branch feel continuous while divergent
branches can never corrupt each other. See the local-preview
[concepts](https://jmelahman.github.io/local-preview/guide/concepts) page
for how that works.

## Onboarding a repo

The board's repo needs a
[`preview.toml`](https://jmelahman.github.io/local-preview/reference/preview-toml)
at its root describing how to build and run it. That's the only setup —
kanban registers the repo with the orchestrator automatically on the first
deploy, and worktree branches are deployable as-is (they share the repo's
object store).

## Using it

Open a ticket's session pane → **previews** tab → **deploy tip**. The deploy
builds the branch's current commit (from the committed tree, never the
working directory) and the row shows status, build logs, and the preview
link once ready. `*.preview.localhost` resolves in every modern browser
with no DNS setup.

Deploys are idempotent per commit — "deploy tip" after new agent commits
builds only what changed.

## Configuration

| Env var | Default | Description |
| --- | --- | --- |
| `KANBAN_PREVIEW_DOMAIN` | `preview.localhost` | Base domain previews are served under. Point a wildcard DNS record at the kanban server to use a real domain. |

Preview state (builds, artifacts, backend state, its own SQLite) lives under
`<data-dir>/previews/`. With `--in-memory` it moves to a temp dir and an
ephemeral DB. If the orchestrator can't start (e.g. `git` missing), kanban
runs normally and the preview endpoints report unavailable.

## Current limitations

- Builds run on the kanban host with the host's toolchains. Building inside
  the board repo's devcontainer (for reproducible builds) is planned — the
  orchestrator already exposes the seam for it.
- Deploys are manual per tip; auto-deploy on agent commits is planned.
