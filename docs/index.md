---
layout: home

hero:
  name: Agentic Kanban
  text: A kanban board for AI agent sessions.
  tagline: Each ticket is bound to an agent session running inside its own git worktree, executed in the target repository's existing devcontainer.
  image:
    light: /demo_light.png
    dark: /demo_dark.png
    alt: Agentic Kanban screenshot
  actions:
    - theme: brand
      text: Get Started
      link: /guide/install
    - theme: alt
      text: Quickstart
      link: /guide/quickstart
    - theme: alt
      text: View on GitHub
      link: https://github.com/jmelahman/agentic-kanban

features:
  - title: Worktree-isolated sessions
    details: Every ticket gets its own git worktree, spun up in the target repo's existing devcontainer — no contention, no rebasing pain.
  - title: Batteries-included devcontainer
    details: Repos without a `.devcontainer/devcontainer.json` fall back to a bundled Ubuntu image with git, gh, node, and Go preinstalled — pinned to the kanban release by digest.
  - title: Pluggable harnesses
    details: Run Claude Code, pi.dev, or another agent harness; switch the active harness from the app's settings.
  - title: REST + CLI + MCP
    details: A small HTTP API for scripting, two convenience CLI subcommands, and a built-in Model Context Protocol server for AI tools.
  - title: Project + user config
    details: Per-repo `.kanban.toml` plus a personal user config; entries merge with sensible last-write-wins semantics.
  - title: GitHub integration
    details: Auto-move tickets when their linked PR or issue changes state; configure column mappings to match your flow.
  - title: One ~20 MB binary
    details: Server, web UI, REST API, CLI, and MCP server all ship in a single self-contained Go binary. No runtime, no node_modules, no sidecar processes — also distributed as a Docker image and a Python wrapper on PyPI.
---
