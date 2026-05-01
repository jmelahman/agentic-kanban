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

func TestLoadDevcontainer_FallbackOrder(t *testing.T) {
	write := func(t *testing.T, path, body string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	t.Run("prefers .devcontainer/devcontainer.json", func(t *testing.T) {
		repo := t.TempDir()
		xdg := t.TempDir()
		t.Setenv("XDG_CONFIG_HOME", xdg)
		write(t, filepath.Join(repo, ".devcontainer", "devcontainer.json"), `{"name":"primary"}`)
		write(t, filepath.Join(repo, ".devcontainer.json"), `{"name":"alt"}`)
		write(t, filepath.Join(xdg, "kanban", "devcontainer.json"), `{"name":"user"}`)

		cfg, err := LoadDevcontainer(repo)
		if err != nil {
			t.Fatalf("LoadDevcontainer: %v", err)
		}
		if cfg.Name != "primary" {
			t.Errorf("Name = %q; want %q", cfg.Name, "primary")
		}
	})

	t.Run("falls back to .devcontainer.json", func(t *testing.T) {
		repo := t.TempDir()
		xdg := t.TempDir()
		t.Setenv("XDG_CONFIG_HOME", xdg)
		write(t, filepath.Join(repo, ".devcontainer.json"), `{"name":"alt"}`)
		write(t, filepath.Join(xdg, "kanban", "devcontainer.json"), `{"name":"user"}`)

		cfg, err := LoadDevcontainer(repo)
		if err != nil {
			t.Fatalf("LoadDevcontainer: %v", err)
		}
		if cfg.Name != "alt" {
			t.Errorf("Name = %q; want %q", cfg.Name, "alt")
		}
	})

	t.Run("falls back to user single-file config when repo has none", func(t *testing.T) {
		repo := t.TempDir()
		xdg := t.TempDir()
		t.Setenv("XDG_CONFIG_HOME", xdg)
		write(t, filepath.Join(xdg, "kanban", "devcontainer.json"), `{"name":"user"}`)

		cfg, err := LoadDevcontainer(repo)
		if err != nil {
			t.Fatalf("LoadDevcontainer: %v", err)
		}
		if cfg.Name != "user" {
			t.Errorf("Name = %q; want %q", cfg.Name, "user")
		}
		wantDir := filepath.Join(xdg, "kanban")
		if cfg.ConfigDir != wantDir {
			t.Errorf("ConfigDir = %q; want %q", cfg.ConfigDir, wantDir)
		}
	})

	t.Run("prefers user .devcontainer/ dir over single-file", func(t *testing.T) {
		repo := t.TempDir()
		xdg := t.TempDir()
		t.Setenv("XDG_CONFIG_HOME", xdg)
		write(t, filepath.Join(xdg, "kanban", ".devcontainer", "devcontainer.json"), `{"name":"user-dir"}`)
		write(t, filepath.Join(xdg, "kanban", "devcontainer.json"), `{"name":"user-file"}`)

		cfg, err := LoadDevcontainer(repo)
		if err != nil {
			t.Fatalf("LoadDevcontainer: %v", err)
		}
		if cfg.Name != "user-dir" {
			t.Errorf("Name = %q; want %q", cfg.Name, "user-dir")
		}
		wantDir := filepath.Join(xdg, "kanban", ".devcontainer")
		if cfg.ConfigDir != wantDir {
			t.Errorf("ConfigDir = %q; want %q", cfg.ConfigDir, wantDir)
		}
	})

	t.Run("errors when neither repo nor user config has one", func(t *testing.T) {
		repo := t.TempDir()
		xdg := t.TempDir()
		t.Setenv("XDG_CONFIG_HOME", xdg)

		if _, err := LoadDevcontainer(repo); err == nil {
			t.Fatal("expected error when no devcontainer.json exists, got nil")
		}
	})
}

func TestResolveBuildPaths(t *testing.T) {
	write := func(t *testing.T, path string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("FROM scratch\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	t.Run("repo .devcontainer/ Dockerfile", func(t *testing.T) {
		repo := t.TempDir()
		write(t, filepath.Join(repo, ".devcontainer", "Dockerfile"))
		cfg := &DevcontainerConfig{ConfigDir: filepath.Join(repo, ".devcontainer")}

		ctxDir, dfPath := resolveBuildPaths(cfg, repo)
		if ctxDir != filepath.Join(repo, ".devcontainer") {
			t.Errorf("contextDir = %q; want %q", ctxDir, filepath.Join(repo, ".devcontainer"))
		}
		if dfPath != filepath.Join(repo, ".devcontainer", "Dockerfile") {
			t.Errorf("dockerfilePath = %q; want %q", dfPath, filepath.Join(repo, ".devcontainer", "Dockerfile"))
		}
	})

	t.Run("repo .devcontainer/ json with repo-root Dockerfile", func(t *testing.T) {
		repo := t.TempDir()
		write(t, filepath.Join(repo, "Dockerfile"))
		cfg := &DevcontainerConfig{ConfigDir: filepath.Join(repo, ".devcontainer")}

		_, dfPath := resolveBuildPaths(cfg, repo)
		if dfPath != filepath.Join(repo, "Dockerfile") {
			t.Errorf("dockerfilePath = %q; want fallback to %q", dfPath, filepath.Join(repo, "Dockerfile"))
		}
	})

	t.Run("user .devcontainer/ Dockerfile resolves under user dir", func(t *testing.T) {
		repo := t.TempDir()
		userDir := t.TempDir()
		userDevcontainer := filepath.Join(userDir, ".devcontainer")
		write(t, filepath.Join(userDevcontainer, "Dockerfile"))
		cfg := &DevcontainerConfig{ConfigDir: userDevcontainer}

		ctxDir, dfPath := resolveBuildPaths(cfg, repo)
		if ctxDir != userDevcontainer {
			t.Errorf("contextDir = %q; want %q", ctxDir, userDevcontainer)
		}
		if dfPath != filepath.Join(userDevcontainer, "Dockerfile") {
			t.Errorf("dockerfilePath = %q; want %q", dfPath, filepath.Join(userDevcontainer, "Dockerfile"))
		}
	})

	t.Run("user single-file json with sibling Dockerfile", func(t *testing.T) {
		repo := t.TempDir()
		userDir := t.TempDir()
		write(t, filepath.Join(userDir, "Dockerfile"))
		cfg := &DevcontainerConfig{ConfigDir: userDir}

		ctxDir, dfPath := resolveBuildPaths(cfg, repo)
		if ctxDir != userDir {
			t.Errorf("contextDir = %q; want %q", ctxDir, userDir)
		}
		if dfPath != filepath.Join(userDir, "Dockerfile") {
			t.Errorf("dockerfilePath = %q; want %q", dfPath, filepath.Join(userDir, "Dockerfile"))
		}
	})

	t.Run("custom dockerfile name and context", func(t *testing.T) {
		userDir := t.TempDir()
		write(t, filepath.Join(userDir, "build", "Dockerfile.dev"))
		cfg := &DevcontainerConfig{
			ConfigDir: userDir,
			Build:     BuildConfig{Dockerfile: "build/Dockerfile.dev", Context: "build"},
		}

		ctxDir, dfPath := resolveBuildPaths(cfg, "/unused/repo")
		if ctxDir != filepath.Join(userDir, "build") {
			t.Errorf("contextDir = %q; want %q", ctxDir, filepath.Join(userDir, "build"))
		}
		if dfPath != filepath.Join(userDir, "build", "Dockerfile.dev") {
			t.Errorf("dockerfilePath = %q; want %q", dfPath, filepath.Join(userDir, "build", "Dockerfile.dev"))
		}
	})
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
