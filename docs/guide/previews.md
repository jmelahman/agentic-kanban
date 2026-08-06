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

## Automatic deploys

When an agent finishes a burst of work (its session transitions to idle),
kanban automatically deploys the branch tip. Deploys are idempotent per
commit, so an unchanged tip is a no-op — and the trigger only fires for
worktrees that carry a `preview.toml`, so boards that haven't onboarded
never accumulate failed deploys. Disable with
`KANBAN_PREVIEW_AUTO_DEPLOY=0`.

## Reproducible builds

Build steps run inside the board repo's **devcontainer** by default: the
devcontainer config is read from the deployed commit's tree (old commits
build with the environment they shipped with), resolved through kanban's
content-addressed image cache (unchanged configs reuse the image), and each
step runs in a one-shot container with the extracted tree mounted. Repos
without a devcontainer build in the builtin session image. Set
`KANBAN_PREVIEW_BUILDS=host` to run build steps directly on the kanban host
instead (or previews fall back to host builds automatically when Docker is
unavailable).

## Configuration

| Env var | Default | Description |
| --- | --- | --- |
| `KANBAN_PREVIEW_DOMAIN` | `preview.localhost` | Base domain previews are served under. Point a wildcard DNS record at the kanban server to use a real domain. |
| `KANBAN_PREVIEW_BUILDS` | `devcontainer` | Set to `host` to run build steps on the kanban host instead of in the repo's devcontainer. |
| `KANBAN_PREVIEW_AUTO_DEPLOY` | on | Set to `0` to disable deploy-on-idle. |

Preview state (builds, artifacts, backend state, its own SQLite) lives under
`<data-dir>/previews/`. With `--in-memory` it moves to a temp dir and an
ephemeral DB. If the orchestrator can't start (e.g. `git` missing), kanban
runs normally and the preview endpoints report unavailable.
