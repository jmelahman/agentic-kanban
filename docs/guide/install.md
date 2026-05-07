# Install

Pick whichever path matches your environment. All three give you the same `kanban` binary; the Docker image bakes it into a long-running container.

## With uv (recommended)

```sh
uv tool install agentic-kanban
```

This installs the binary to `~/.local/bin/kanban`. Make sure that's on your `PATH`.

## With Docker

The Docker image is the most "batteries-included" option — it ships with a Docker socket mount so kanban can spawn session containers, and it pre-mounts `~/.claude` so the Claude Code harness picks up your existing credentials.

```sh
SOURCE=$HOME/code
docker run -d --name kanban \
  --restart unless-stopped \
  -p 127.0.0.1:7474:7474 \
  -p 13000-13099:13000-13099 \
  -v ${DOCKER_SOCK_PATH:-/var/run/docker.sock}:/var/run/docker.sock \
  -v $HOME/.claude:$HOME/.claude \
  -v $HOME/.local/share/kanban:$HOME/.local/share/kanban \
  -v $SOURCE:$SOURCE \
  -e HOME=$HOME \
  -e XDG_RUNTIME_DIR=$XDG_RUNTIME_DIR \
  -e KANBAN_DATA_DIR=$HOME/.local/share/kanban \
  -e GH_TOKEN=$(gh auth token) \
  lahmanja/kanban:latest
```

::: tip Rootless Docker
Set `DOCKER_SOCK_PATH` in your shell to point at your rootless socket — see the [rootless Docker docs](https://docs.docker.com/engine/security/rootless/).
:::

## From GitHub Releases

Prebuilt Linux/macOS/Windows binaries are attached to every tag on the [Releases page](https://github.com/jmelahman/agentic-kanban/releases). Drop the binary somewhere on your `PATH`.

## Build from source

You'll need Go 1.22+ and Node 24+.

```sh
git clone https://github.com/jmelahman/agentic-kanban
cd agentic-kanban
docker bake          # builds the multi-arch container image
# ...or, for a local Go build:
npm --prefix web ci && npm --prefix web run build
go build -o kanban
```

## Verify

```sh
kanban --version
kanban serve --in-memory
```

The server logs `WARNING: --in-memory set` and starts listening on `:7474`. Open <http://localhost:7474/> — you should see the kanban UI.

Press <kbd>Ctrl</kbd>+<kbd>C</kbd> to shut down. With `--in-memory` everything you created is discarded.
