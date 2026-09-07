package api

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/jmelahman/kanban/internal/config"
)

func TestGuardKeyWorktreesLock(t *testing.T) {
	dir := t.TempDir()
	locked, err := config.Load(dir, filepath.Join(dir, "wt"), 13000, 13099)
	if err != nil {
		t.Fatal(err)
	}
	h := &handlers{config: locked}
	if status, err := h.guardKey("worktrees.root", "x"); err == nil || status != http.StatusConflict {
		t.Fatalf("locked worktrees: status=%d err=%v, want 409", status, err)
	}

	unlocked, err := config.Load(dir, "", 13000, 13099)
	if err != nil {
		t.Fatal(err)
	}
	hu := &handlers{config: unlocked}
	if status, err := hu.guardKey("worktrees.root", "x"); err != nil || status != 0 {
		t.Fatalf("unlocked worktrees: status=%d err=%v, want ok", status, err)
	}
}

func TestGuardKeyHarness(t *testing.T) {
	h := &handlers{}
	if status, err := h.guardKey("harness.id", "definitely-not-a-harness"); err == nil || status != 400 {
		t.Fatalf("unknown harness: status=%d err=%v, want 400", status, err)
	}
	// Empty id (clearing the key) is always allowed.
	if status, err := h.guardKey("harness.id", ""); err != nil || status != 0 {
		t.Fatalf("empty harness: status=%d err=%v, want ok", status, err)
	}
}

func TestLoadMergeConfigDefaultStrategy(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	repo := t.TempDir()
	if got := loadMergeConfig(repo).DefaultStrategy; got != "" {
		t.Errorf("no config: DefaultStrategy = %q; want empty", got)
	}
	toml := "[merge]\ndefault_strategy = \"rebase\"\nallow_squash = false\n"
	if err := os.WriteFile(filepath.Join(repo, ".kanban.toml"), []byte(toml), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := loadMergeConfig(repo)
	if cfg.DefaultStrategy != "rebase" {
		t.Errorf("DefaultStrategy = %q; want %q", cfg.DefaultStrategy, "rebase")
	}
	if !cfg.allows(cfg.DefaultStrategy) {
		t.Error("configured default strategy should be allowed")
	}
	if cfg.AllowSquash {
		t.Error("allow_squash = true; want the config's false")
	}
}
