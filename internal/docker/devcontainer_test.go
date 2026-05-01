package docker

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/docker/docker/api/types/mount"
)

func TestSubstitute(t *testing.T) {
	t.Setenv("KANBAN_TEST_SET", "world")
	t.Setenv("KANBAN_TEST_EMPTY", "")

	ctx := NewSubstitutionContext("/host/onyx", "/workspace")

	cases := []struct {
		name string
		in   string
		want string
	}{
		{"plain", "hello", "hello"},
		{"localEnv set", "${localEnv:KANBAN_TEST_SET}", "world"},
		{"localEnv unset no default", "${localEnv:KANBAN_TEST_UNSET}", ""},
		{"localEnv unset with default", "${localEnv:KANBAN_TEST_UNSET:dev}", "dev"},
		{"localEnv set ignores default", "${localEnv:KANBAN_TEST_SET:dev}", "world"},
		{"localEnv empty-but-set keeps empty", "${localEnv:KANBAN_TEST_EMPTY:dev}", ""},
		{"localWorkspaceFolder", "${localWorkspaceFolder}/sub", "/host/onyx/sub"},
		{"localWorkspaceFolderBasename", "${localWorkspaceFolderBasename}", "onyx"},
		{"containerWorkspaceFolder", "${containerWorkspaceFolder}", "/workspace"},
		{"containerWorkspaceFolderBasename", "${containerWorkspaceFolderBasename}", "workspace"},
		{"containerEnv left literal", "${containerEnv:PATH}", "${containerEnv:PATH}"},
		{"unknown var left literal", "${notARealVar}", "${notARealVar}"},
		{"multiple vars in one string",
			"user=${localEnv:KANBAN_TEST_SET},dir=${containerWorkspaceFolder}",
			"user=world,dir=/workspace"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Substitute(tc.in, ctx); got != tc.want {
				t.Errorf("Substitute(%q) = %q; want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestSubstitute_DevcontainerIDIsStable(t *testing.T) {
	a := NewSubstitutionContext("/foo", "/workspace").DevcontainerID
	b := NewSubstitutionContext("/foo", "/workspace").DevcontainerID
	c := NewSubstitutionContext("/bar", "/workspace").DevcontainerID
	if a != b {
		t.Errorf("devcontainerId not stable for same worktree: %q vs %q", a, b)
	}
	if a == c {
		t.Errorf("devcontainerId collided across worktrees: %q", a)
	}
}

func TestDevcontainerConfig_Substitute(t *testing.T) {
	t.Setenv("KANBAN_TEST_USER", "root")
	t.Setenv("KANBAN_TEST_HOME", "/h")

	cfg := &DevcontainerConfig{
		Name:  "${localEnv:KANBAN_TEST_USER}-box",
		Image: "img",
		Build: BuildConfig{
			Args: map[string]string{"USER": "${localEnv:KANBAN_TEST_USER}"},
		},
		RunArgs: []string{"--name", "${localEnv:KANBAN_TEST_USER}"},
		Mounts: []string{
			"source=${localEnv:KANBAN_TEST_HOME}/.claude,target=/root/.claude,type=bind",
		},
		WorkspaceMount:   "source=${localWorkspaceFolder},target=${containerWorkspaceFolder}",
		WorkspaceFolder:  "/workspace",
		RemoteUser:       "${localEnv:KANBAN_TEST_USER:dev}",
		ContainerEnv:     map[string]string{"GH_TOKEN": "${localEnv:KANBAN_TEST_HOME}"},
		PostStartCommand: "echo ${localWorkspaceFolderBasename}",
	}

	cfg.Substitute(NewSubstitutionContext("/host/proj", cfg.WorkspaceFolder))

	want := &DevcontainerConfig{
		Name:  "root-box",
		Image: "img",
		Build: BuildConfig{
			Args: map[string]string{"USER": "root"},
		},
		RunArgs: []string{"--name", "root"},
		Mounts: []string{
			"source=/h/.claude,target=/root/.claude,type=bind",
		},
		WorkspaceMount:   "source=/host/proj,target=/workspace",
		WorkspaceFolder:  "/workspace",
		RemoteUser:       "root",
		ContainerEnv:     map[string]string{"GH_TOKEN": "/h"},
		PostStartCommand: "echo proj",
	}

	if !reflect.DeepEqual(cfg, want) {
		t.Errorf("Substitute mismatch.\n got: %#v\nwant: %#v", cfg, want)
	}
}

func TestDevcontainerConfig_Substitute_RemoteUserDefault(t *testing.T) {
	// Reproduces the original bug: ${localEnv:VAR:default} in remoteUser was
	// passed verbatim to Docker, causing "unable to find user ${localEnv:".
	cfg := &DevcontainerConfig{RemoteUser: "${localEnv:DEVCONTAINER_REMOTE_USER:dev}"}
	cfg.Substitute(NewSubstitutionContext("/x", "/workspace"))
	if cfg.RemoteUser != "dev" {
		t.Errorf("RemoteUser = %q; want %q (default applied)", cfg.RemoteUser, "dev")
	}

	t.Setenv("DEVCONTAINER_REMOTE_USER", "root")
	cfg = &DevcontainerConfig{RemoteUser: "${localEnv:DEVCONTAINER_REMOTE_USER:dev}"}
	cfg.Substitute(NewSubstitutionContext("/x", "/workspace"))
	if cfg.RemoteUser != "root" {
		t.Errorf("RemoteUser = %q; want %q (env var applied)", cfg.RemoteUser, "root")
	}
}

func TestBuildContainerConfig_SourceRepoGitMount(t *testing.T) {
	// Simulate a parent repo on disk with a real .git directory; the worktree's
	// gitdir pointer references this absolute path, so we bind-mount it as-is
	// into the container.
	repo := t.TempDir()
	gitDir := filepath.Join(repo, ".git")
	if err := os.MkdirAll(gitDir, 0o755); err != nil {
		t.Fatal(err)
	}

	cfg := &DevcontainerConfig{WorkspaceFolder: "/workspace"}
	opts := SpawnOptions{
		WorktreePath:   "/host/worktree",
		SourceRepoPath: repo,
		ContainerName:  "test",
	}

	hostCfg, _, _, err := buildContainerConfig(cfg, opts, "img", "")
	if err != nil {
		t.Fatalf("buildContainerConfig: %v", err)
	}

	var found bool
	for _, m := range hostCfg.Mounts {
		if m.Type == mount.TypeBind && m.Source == gitDir && m.Target == gitDir {
			found = true
			break
		}
	}
	if !found {
		t.Errorf(".git bind mount missing.\n got mounts: %#v\n want bind src=target=%q", hostCfg.Mounts, gitDir)
	}
}

func TestBuildContainerConfig_NoGitMountWhenSourceMissing(t *testing.T) {
	cfg := &DevcontainerConfig{WorkspaceFolder: "/workspace"}
	opts := SpawnOptions{WorktreePath: "/host/worktree", ContainerName: "test"}

	hostCfg, _, _, err := buildContainerConfig(cfg, opts, "img", "")
	if err != nil {
		t.Fatalf("buildContainerConfig: %v", err)
	}

	if len(hostCfg.Mounts) != 1 {
		t.Errorf("expected only the workspace mount; got %#v", hostCfg.Mounts)
	}
}

func TestBuildContainerConfig_HostDockerInternalAlias(t *testing.T) {
	cfg := &DevcontainerConfig{WorkspaceFolder: "/workspace"}
	opts := SpawnOptions{WorktreePath: "/host/worktree", ContainerName: "test"}

	cases := []struct {
		name      string
		gatewayIP string
		want      string
	}{
		{"falls back to host-gateway", "", "host.docker.internal:host-gateway"},
		{"uses explicit gateway IP", "172.19.0.1", "host.docker.internal:172.19.0.1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			hostCfg, _, _, err := buildContainerConfig(cfg, opts, "img", tc.gatewayIP)
			if err != nil {
				t.Fatalf("buildContainerConfig: %v", err)
			}
			if !reflect.DeepEqual(hostCfg.ExtraHosts, []string{tc.want}) {
				t.Errorf("ExtraHosts = %v; want [%q]", hostCfg.ExtraHosts, tc.want)
			}
		})
	}
}
