# Agentic Kanban

[![Test status](https://github.com/jmelahman/agentic-kanban/actions/workflows/test.yml/badge.svg)](https://github.com/jmelahman/agentic-kanban/actions)
[![Deploy Status](https://github.com/jmelahman/agentic-kanban/actions/workflows/release.yml/badge.svg)](https://github.com/jmelahman/agentic-kanban/actions)
[![Docs](https://github.com/jmelahman/agentic-kanban/actions/workflows/docs.yml/badge.svg)](https://jmelahman.github.io/agentic-kanban/)
[![Go Reference](https://pkg.go.dev/badge/github.com/jmelahman/agentic-kanban.svg)](https://pkg.go.dev/github.com/jmelahman/agentic-kanban)
[![PyPI](https://img.shields.io/pypi/v/agentic-kanban.svg)](https://pypi.org/project/agentic-kanban/)

A kanban board for managing AI agent sessions.

<p align="center">
  <picture align="center">
    <source media="(prefers-color-scheme: dark)" srcset="https://github.com/jmelahman/agentic-kanban/blob/master/assets/demo_dark.png">
    <source media="(prefers-color-scheme: light)" srcset="https://github.com/jmelahman/agentic-kanban/blob/master/assets/demo_light.png">
    <img src="https://github.com/jmelahman/agentic-kanban/blob/master/assets/demo_light.png">
  </picture>
</p>

Each ticket is bound to an agent session (Claude Code, pi.dev) running inside its own git worktree, executed in the target repository's existing devcontainer. The active harness is selected globally in the app's settings.

## Install

```sh
uv tool install agentic-kanban
```

Docker images and prebuilt binaries are also available — see the [install guide](https://jmelahman.github.io/agentic-kanban/guide/install).

## Run

```sh
kanban serve
```

The server listens on `:7474`. Open <http://localhost:7474/>.

Or with Docker:

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

## 📖 Documentation

Full documentation lives at **<https://jmelahman.github.io/agentic-kanban/>**:

- [Quickstart](https://jmelahman.github.io/agentic-kanban/guide/quickstart) — your first board and ticket in five minutes.
- [Configuration](https://jmelahman.github.io/agentic-kanban/guide/configuration) — `.kanban.toml` schema and merge rules.
- [REST API](https://jmelahman.github.io/agentic-kanban/reference/api) — endpoints exposed by the running server.
- [CLI](https://jmelahman.github.io/agentic-kanban/reference/cli) — `serve`, `mcp`, `list-boards`, `create-ticket`.
- [MCP](https://jmelahman.github.io/agentic-kanban/reference/mcp) — Claude Desktop / Claude Code integration.
