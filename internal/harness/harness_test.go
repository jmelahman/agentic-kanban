package harness

import (
	"strings"
	"testing"
)

func TestRenderStartScript_Claude(t *testing.T) {
	h := Get("claude")
	script, err := h.RenderStartScript("/workspace/.kanban/prompt.txt")
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if script == "" {
		t.Fatal("expected non-empty script for claude harness")
	}
	if !strings.Contains(script, "'/workspace/.kanban/prompt.txt'") {
		t.Errorf("prompt path should be shell-quoted in script:\n%s", script)
	}
	if !strings.Contains(script, "claude -p") {
		t.Errorf("script should invoke claude -p; got:\n%s", script)
	}
	if !strings.Contains(script, "--dangerously-skip-permissions") {
		t.Errorf("script should pass --dangerously-skip-permissions; got:\n%s", script)
	}
	if !strings.Contains(script, "nohup") || !strings.HasSuffix(strings.TrimSpace(script), "&") {
		t.Errorf("script should background the agent (nohup ... &); got:\n%s", script)
	}
}

func TestRenderStartScript_QuotesAdversarialPath(t *testing.T) {
	h := Get("claude")
	script, err := h.RenderStartScript(`/tmp/it's a path.txt`)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	// ShellQuote turns a single quote into '\'' inside a single-quoted span.
	if !strings.Contains(script, `'/tmp/it'\''s a path.txt'`) {
		t.Errorf("quoting did not escape single quote; got:\n%s", script)
	}
}

func TestRenderStartScript_EmptyTemplateReturnsEmpty(t *testing.T) {
	h := Get("pi") // pi has no StartCommandTemplate
	script, err := h.RenderStartScript("/workspace/.kanban/prompt.txt")
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if script != "" {
		t.Errorf("expected empty script when template is empty; got %q", script)
	}
}
