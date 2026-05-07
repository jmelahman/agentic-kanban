---
layout: home

hero:
  name: Agentic Kanban
  text: A kanban board for AI agent sessions.
  tagline: Each ticket is bound to an agent session running inside its own git worktree, executed in the target repository's existing devcontainer.
  image:
    src: /demo_light.png
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
  - title: Pluggable harnesses
    details: Run Claude Code, pi.dev, or another agent harness; switch the active harness from the app's settings.
  - title: REST + CLI + MCP
    details: A small HTTP API for scripting, two convenience CLI subcommands, and a built-in Model Context Protocol server for AI tools.
  - title: Project + user config
    details: Per-repo `.kanban.toml` plus a personal user config; entries merge with sensible last-write-wins semantics.
  - title: GitHub integration
    details: Auto-move tickets when their linked PR or issue changes state; configure column mappings to match your flow.
  - title: One binary
    details: Ships as a Go binary, a Docker image, and a Python wrapper on PyPI. Install whichever fits your machine.
---
