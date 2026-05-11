package docker

import (
	"os"
	"path/filepath"
	"runtime"
)

// BuiltinImage is the Docker image reference for the bundled session
// devcontainer. Release builds override this via -ldflags so each kanban
// version pins to the exact image digest published alongside the binary:
//
//	-ldflags "-X github.com/jmelahman/kanban/internal/docker.BuiltinImage=lahmanja/kanban-devcontainer@sha256:..."
//
// The default points at the floating :latest tag for unpinned development
// builds; tag releases must always set the digest form.
var BuiltinImage = "lahmanja/kanban-devcontainer:latest"

// BuiltinDevcontainer returns the bundled DevcontainerConfig used when a
// session has no repo-level or user-level devcontainer.json. It is
// deliberately permissive: SSH agent and gh credentials are forwarded
// from the host so that git, ssh, and gh "just work" on a fresh install.
// Host Claude Code config is layered on later by the session manager
// (see ClaudeConfigMounts) so it can be toggled via .kanban.toml.
func BuiltinDevcontainer() *DevcontainerConfig {
	cfg := &DevcontainerConfig{
		Name:            "kanban-default",
		Image:           BuiltinImage,
		WorkspaceFolder: "/workspace",
		RemoteUser:      "dev",
		BuiltIn:         true,
		ContainerEnv:    map[string]string{},
	}
	if sock := sshAgentSocketPath(); sock != "" {
		cfg.Mounts = append(cfg.Mounts,
			"type=bind,source="+sock+",target=/ssh-agent.sock")
		cfg.ContainerEnv["SSH_AUTH_SOCK"] = "/ssh-agent.sock"
	}
	if tok := os.Getenv("GH_TOKEN"); tok != "" {
		cfg.ContainerEnv["GH_TOKEN"] = tok
	}
	return cfg
}

// ClaudeConfigMounts returns bind-mount strings that forward the host's
// Claude Code config (~/.claude and ~/.claude.json) into the built-in
// container's home directory so credentials and session history persist
// across sessions. Sources that don't exist on the host are skipped so
// users without Claude Code installed aren't broken. The session manager
// applies these to built-in configs unless the .kanban.toml
// [devcontainer].claude_config flag is set to false.
func ClaudeConfigMounts() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	var mounts []string
	for _, name := range []string{".claude", ".claude.json"} {
		src := filepath.Join(home, name)
		if _, err := os.Stat(src); err != nil {
			continue
		}
		mounts = append(mounts, "type=bind,source="+src+",target=/home/dev/"+name)
	}
	return mounts
}

// DockerSocketMount returns the host docker socket bind mount string applied
// to built-in configs when DevcontainerSection.DockerSocket is unset or true.
// It prefers $KANBAN_HOST_DOCKER_SOCK when set (for kanban-in-a-container
// installs where the local probe would resolve a path that's invalid on the
// host), then probes /var/run/docker.sock, then $XDG_RUNTIME_DIR/docker.sock
// for rootless installs. Returns "" when nothing is set and neither candidate
// exists. Hand-written devcontainer.json files manage their own socket mount
// and are unaffected by the flag.
func DockerSocketMount() string {
	if src := os.Getenv(envHostDockerSock); src != "" {
		return "type=bind,source=" + src + ",target=/var/run/docker.sock"
	}
	for _, src := range dockerSocketCandidates() {
		if _, err := os.Stat(src); err == nil {
			return "type=bind,source=" + src + ",target=/var/run/docker.sock"
		}
	}
	return ""
}

func dockerSocketCandidates() []string {
	paths := []string{"/var/run/docker.sock"}
	if xdg := os.Getenv("XDG_RUNTIME_DIR"); xdg != "" {
		paths = append(paths, filepath.Join(xdg, "docker.sock"))
	}
	return paths
}

// sshAgentSocketPath returns a host path to bind-mount as the container's
// ssh-agent socket. Returns SSH_AUTH_SOCK when set, falling back to the
// Docker Desktop magic socket on macOS so users with launchd-managed agents
// get forwarding without manual env wiring.
func sshAgentSocketPath() string {
	if s := os.Getenv("SSH_AUTH_SOCK"); s != "" {
		return s
	}
	if runtime.GOOS == "darwin" {
		return "/run/host-services/ssh-auth.sock"
	}
	return ""
}
