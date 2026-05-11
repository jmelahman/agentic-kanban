// Package harness describes the AI agent CLIs the kanban server can attach to.
// Each Harness names a command to spawn for the interactive PTY and an optional
// shell template for non-interactive commit-message generation.
package harness

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"
)

type Harness struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	// PTYCommand is the argv exec'd inside the devcontainer for the interactive
	// terminal session.
	PTYCommand []string `json:"pty_command"`
	// CommitMsgTemplate is a sh(1) script template for one-shot commit-message
	// generation. The placeholder {{.Prompt}} is replaced with a shell-quoted
	// prompt before execution. An empty value disables AI commit messages for
	// this harness, falling back to the ticket title.
	CommitMsgTemplate string `json:"-"`
	// StartCommandTemplate is a sh(1) script template for kicking off the agent
	// non-interactively when a board opts into [agent].auto_start. The
	// placeholder {{.PromptFile}} is replaced with a shell-quoted path to a
	// file inside the container holding the ticket body. Templates are
	// expected to background the agent (`nohup ... &`) so session start does
	// not block. An empty value disables auto-start for this harness.
	StartCommandTemplate string `json:"-"`
}

// Registry is the ordered list of supported harnesses. The first entry is the
// default. Add a new harness by appending to this slice.
var Registry = []Harness{
	{
		ID:                "claude",
		Label:             "Claude Code",
		PTYCommand:        []string{"claude"},
		CommitMsgTemplate: `cd /workspace && git diff --staged --no-color | claude --model haiku -p {{.Prompt}}`,
		// Background the agent and detach stdio so this exec returns
		// immediately — start_session must not block on the loop.
		// --dangerously-skip-permissions is required because there is no
		// human at the PTY to approve tool calls; the session container is
		// the isolation boundary. IS_SANDBOX=1 is Claude Code's opt-in to
		// allow --dangerously-skip-permissions while running as root (the
		// devcontainer runs as uid 0 to support rootless dockerd on the
		// host); without it claude exits with "cannot be used with
		// root/sudo privileges".
		StartCommandTemplate: `cd /workspace && mkdir -p .kanban && IS_SANDBOX=1 nohup claude -p "$(cat {{.PromptFile}})" --dangerously-skip-permissions >.kanban/agent.log 2>&1 </dev/null &`,
	},
	{
		ID:                "pi",
		Label:             "pi (Pi Labs / Ollama)",
		PTYCommand:        []string{"pi"},
		CommitMsgTemplate: ``,
	},
}

func Default() Harness { return Registry[0] }

func Get(id string) Harness {
	for _, h := range Registry {
		if h.ID == id {
			return h
		}
	}
	return Default()
}

func IsKnown(id string) bool {
	for _, h := range Registry {
		if h.ID == id {
			return true
		}
	}
	return false
}

// RenderCommitScript returns the shell script to execute for commit-message
// generation, with prompt shell-quoted into the {{.Prompt}} placeholder.
// Returns "" when this harness has no template (caller should fall back).
func (h Harness) RenderCommitScript(prompt string) (string, error) {
	if h.CommitMsgTemplate == "" {
		return "", nil
	}
	t, err := template.New("commit").Parse(h.CommitMsgTemplate)
	if err != nil {
		return "", fmt.Errorf("parse commit template: %w", err)
	}
	var b bytes.Buffer
	if err := t.Execute(&b, struct{ Prompt string }{Prompt: ShellQuote(prompt)}); err != nil {
		return "", fmt.Errorf("render commit template: %w", err)
	}
	return b.String(), nil
}

// RenderStartScript returns the shell script to execute for auto-starting the
// agent, with promptFile shell-quoted into the {{.PromptFile}} placeholder.
// Returns "" when this harness has no template (caller should skip auto-start).
func (h Harness) RenderStartScript(promptFile string) (string, error) {
	if h.StartCommandTemplate == "" {
		return "", nil
	}
	t, err := template.New("start").Parse(h.StartCommandTemplate)
	if err != nil {
		return "", fmt.Errorf("parse start template: %w", err)
	}
	var b bytes.Buffer
	if err := t.Execute(&b, struct{ PromptFile string }{PromptFile: ShellQuote(promptFile)}); err != nil {
		return "", fmt.Errorf("render start template: %w", err)
	}
	return b.String(), nil
}

func ShellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
