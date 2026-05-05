package session

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jmelahman/kanban/internal/db"
	"github.com/jmelahman/kanban/internal/docker"
	"github.com/jmelahman/kanban/internal/kanbantoml"
)

func TestResolveBranchPrefix(t *testing.T) {
	// Neutralise the user-level config so tests don't pick up the dev
	// machine's ~/.config/kanban/config.toml.
	t.Setenv("KANBAN_CONFIG", filepath.Join(t.TempDir(), "missing.toml"))

	t.Run("board override wins", func(t *testing.T) {
		repoDir := t.TempDir()
		writeProjectToml(t, repoDir, `[branches]
prefix = "proj/"`)
		got := resolveBranchPrefix(&db.Board{Slug: "demo", BranchPrefix: "feat"}, repoDir)
		if got != "feat" {
			t.Errorf("got %q; want feat", got)
		}
	})

	t.Run("toml fallback when board empty", func(t *testing.T) {
		repoDir := t.TempDir()
		writeProjectToml(t, repoDir, `[branches]
prefix = "proj"`)
		got := resolveBranchPrefix(&db.Board{Slug: "demo"}, repoDir)
		if got != "proj" {
			t.Errorf("got %q; want proj", got)
		}
	})

	t.Run("hardcoded default when neither set", func(t *testing.T) {
		got := resolveBranchPrefix(&db.Board{Slug: "demo"}, t.TempDir())
		if got != "kanban/demo" {
			t.Errorf("got %q; want kanban/demo", got)
		}
	})

	t.Run("whitespace-only board prefix falls through", func(t *testing.T) {
		got := resolveBranchPrefix(&db.Board{Slug: "demo", BranchPrefix: "   "}, t.TempDir())
		if got != "kanban/demo" {
			t.Errorf("got %q; want kanban/demo", got)
		}
	})
}

func writeProjectToml(t *testing.T, dir, body string) {
	t.Helper()
	path := filepath.Join(dir, ".kanban.toml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestApplyKanbanDevcontainerOverrides_AppendsAndMergesEnv(t *testing.T) {
	cfg := &docker.DevcontainerConfig{
		RunArgs: []string{"--cap-add=NET_ADMIN"},
		Mounts:  []string{"type=volume,source=cache,target=/cache"},
		ContainerEnv: map[string]string{
			"LANG": "C.UTF-8",
			"TZ":   "UTC",
		},
	}
	dev := &kanbantoml.DevcontainerSection{
		RunArgs: []string{"--cap-add=SYS_PTRACE"},
		Mounts:  []string{"type=bind,source=/tmp/ssh-agent.sock,target=/tmp/ssh-agent.sock"},
		ContainerEnv: map[string]string{
			"LANG":          "en_US.UTF-8",
			"SSH_AUTH_SOCK": "/tmp/ssh-agent.sock",
		},
	}

	applyKanbanDevcontainerOverrides(cfg, dev)

	if got := len(cfg.RunArgs); got != 2 {
		t.Errorf("len(RunArgs) = %d; want 2", got)
	}
	if cfg.RunArgs[1] != "--cap-add=SYS_PTRACE" {
		t.Errorf("RunArgs[1] = %q; want --cap-add=SYS_PTRACE", cfg.RunArgs[1])
	}
	if got := len(cfg.Mounts); got != 2 {
		t.Errorf("len(Mounts) = %d; want 2", got)
	}
	if cfg.ContainerEnv["LANG"] != "en_US.UTF-8" {
		t.Errorf("ContainerEnv[LANG] = %q; want en_US.UTF-8 (kanban override)", cfg.ContainerEnv["LANG"])
	}
	if cfg.ContainerEnv["TZ"] != "UTC" {
		t.Errorf("ContainerEnv[TZ] = %q; want UTC (preserved)", cfg.ContainerEnv["TZ"])
	}
	if cfg.ContainerEnv["SSH_AUTH_SOCK"] != "/tmp/ssh-agent.sock" {
		t.Errorf("ContainerEnv[SSH_AUTH_SOCK] = %q; want /tmp/ssh-agent.sock", cfg.ContainerEnv["SSH_AUTH_SOCK"])
	}
}

func TestApplyKanbanDevcontainerOverrides_NilDev(t *testing.T) {
	cfg := &docker.DevcontainerConfig{RunArgs: []string{"--privileged"}}
	applyKanbanDevcontainerOverrides(cfg, nil)
	if len(cfg.RunArgs) != 1 || cfg.RunArgs[0] != "--privileged" {
		t.Errorf("RunArgs mutated by nil dev: %v", cfg.RunArgs)
	}
}

func TestApplyKanbanDevcontainerOverrides_InitializesEnvMap(t *testing.T) {
	cfg := &docker.DevcontainerConfig{}
	dev := &kanbantoml.DevcontainerSection{
		ContainerEnv: map[string]string{"FOO": "bar"},
	}
	applyKanbanDevcontainerOverrides(cfg, dev)
	if cfg.ContainerEnv["FOO"] != "bar" {
		t.Errorf("ContainerEnv[FOO] = %q; want bar", cfg.ContainerEnv["FOO"])
	}
}
