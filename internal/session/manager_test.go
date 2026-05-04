package session

import (
	"testing"

	"github.com/jmelahman/kanban/internal/docker"
	"github.com/jmelahman/kanban/internal/kanbantoml"
)

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
