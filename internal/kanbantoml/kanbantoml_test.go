package kanbantoml

import (
	"os"
	"path/filepath"
	"testing"
)

// withUserConfig redirects the user-config lookup at $XDG_CONFIG_HOME and
// writes contents to that location, returning the temp dir for cleanup hooks.
// An empty contents string skips writing the file (so absence is testable).
func withUserConfig(t *testing.T, contents string) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	if contents == "" {
		return
	}
	cfgDir := filepath.Join(dir, "kanban")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatalf("mkdir user config dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "config.toml"), []byte(contents), 0o644); err != nil {
		t.Fatalf("write user config: %v", err)
	}
}

func writeProject(t *testing.T, contents string) string {
	t.Helper()
	dir := t.TempDir()
	if contents != "" {
		if err := os.WriteFile(filepath.Join(dir, ".kanban.toml"), []byte(contents), 0o644); err != nil {
			t.Fatalf("write project config: %v", err)
		}
	}
	return dir
}

func TestLoad_UserOverridesProject_Sync(t *testing.T) {
	withUserConfig(t, `[sync]
allow_rebase = false
`)
	repo := writeProject(t, `[sync]
allow_rebase = true
allow_merge = false
`)

	f := Load(repo)
	if f.Sync == nil {
		t.Fatal("sync section missing")
	}
	if f.Sync.AllowRebase == nil || *f.Sync.AllowRebase {
		t.Errorf("allow_rebase = %v; want false (user override)", f.Sync.AllowRebase)
	}
	if f.Sync.AllowMerge == nil || *f.Sync.AllowMerge {
		t.Errorf("allow_merge = %v; want false (project preserved)", f.Sync.AllowMerge)
	}
}

func TestLoad_UserOverridesProject_Merge(t *testing.T) {
	withUserConfig(t, `[merge]
allow_merge_commit = true
`)
	repo := writeProject(t, `[merge]
allow_merge_commit = false
allow_squash = true
allow_rebase = false
`)

	f := Load(repo)
	if f.Merge == nil {
		t.Fatal("merge section missing")
	}
	if f.Merge.AllowMergeCommit == nil || !*f.Merge.AllowMergeCommit {
		t.Errorf("allow_merge_commit = %v; want true (user override)", f.Merge.AllowMergeCommit)
	}
	if f.Merge.AllowSquash == nil || !*f.Merge.AllowSquash {
		t.Errorf("allow_squash = %v; want true (project preserved)", f.Merge.AllowSquash)
	}
	if f.Merge.AllowRebase == nil || *f.Merge.AllowRebase {
		t.Errorf("allow_rebase = %v; want false (project preserved)", f.Merge.AllowRebase)
	}
}

func TestLoad_UserOverridesProject_Harness(t *testing.T) {
	withUserConfig(t, `[harness]
id = "pi"
`)
	repo := writeProject(t, `[harness]
id = "claude"
`)

	f := Load(repo)
	if f.Harness == nil || f.Harness.ID == nil {
		t.Fatal("harness id missing")
	}
	if *f.Harness.ID != "pi" {
		t.Errorf("harness id = %q; want \"pi\"", *f.Harness.ID)
	}
}

func TestLoad_UserOnly(t *testing.T) {
	withUserConfig(t, `[github]
auto_move = true
draft_column = "WIP"
`)
	repo := writeProject(t, ``)

	f := Load(repo)
	if f.GitHub == nil {
		t.Fatal("github section missing")
	}
	if f.GitHub.AutoMove == nil || !*f.GitHub.AutoMove {
		t.Errorf("auto_move = %v; want true", f.GitHub.AutoMove)
	}
	if f.GitHub.DraftColumn == nil || *f.GitHub.DraftColumn != "WIP" {
		t.Errorf("draft_column = %v; want WIP", f.GitHub.DraftColumn)
	}
}

func TestLoad_ProjectOnly(t *testing.T) {
	withUserConfig(t, "")
	repo := writeProject(t, `[merge]
allow_squash = false
`)

	f := Load(repo)
	if f.Merge == nil || f.Merge.AllowSquash == nil || *f.Merge.AllowSquash {
		t.Errorf("allow_squash = %v; want false", f.Merge.AllowSquash)
	}
}

func TestLoad_TasksMergeByLabel(t *testing.T) {
	withUserConfig(t, `[[task]]
label = "Frontend"
container_port = 8080

[[task]]
label = "Tests"
container_port = 9000
`)
	repo := writeProject(t, `[[task]]
label = "Backend"
container_port = 7474

[[task]]
label = "Frontend"
container_port = 5173
`)

	f := Load(repo)
	if got := len(f.Tasks); got != 3 {
		t.Fatalf("len(tasks) = %d; want 3 (Backend, Frontend, Tests)", got)
	}

	port, ok := f.PortFor("Backend")
	if !ok || port != 7474 {
		t.Errorf("Backend port = %d, ok = %v; want 7474, true", port, ok)
	}
	port, ok = f.PortFor("Frontend")
	if !ok || port != 8080 {
		t.Errorf("Frontend port = %d, ok = %v; want 8080 (user override), true", port, ok)
	}
	port, ok = f.PortFor("Tests")
	if !ok || port != 9000 {
		t.Errorf("Tests port = %d, ok = %v; want 9000 (user-only), true", port, ok)
	}
}

func TestLoad_NoFiles(t *testing.T) {
	withUserConfig(t, "")
	repo := writeProject(t, "")

	f := Load(repo)
	if f.Harness != nil || f.Sync != nil || f.Merge != nil || f.GitHub != nil || f.Tasks != nil {
		t.Errorf("expected fully empty File, got %+v", f)
	}
}

func TestWriteUserHarness_RoundTrip(t *testing.T) {
	withUserConfig(t, "")

	if err := WriteUserHarness("pi"); err != nil {
		t.Fatalf("write: %v", err)
	}
	f := Load("")
	if f.Harness == nil || f.Harness.ID == nil || *f.Harness.ID != "pi" {
		t.Fatalf("after write, harness id = %v; want pi", f.Harness)
	}

	// Clearing removes the file when nothing else is in it.
	if err := WriteUserHarness(""); err != nil {
		t.Fatalf("clear: %v", err)
	}
	path, _ := UserPath()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("user config still exists after clear: err=%v", err)
	}
}

func TestWriteUserHarness_PreservesOtherKeys(t *testing.T) {
	withUserConfig(t, `[sync]
allow_rebase = false
`)

	if err := WriteUserHarness("claude"); err != nil {
		t.Fatalf("write: %v", err)
	}
	f := Load("")
	if f.Harness == nil || f.Harness.ID == nil || *f.Harness.ID != "claude" {
		t.Errorf("harness id = %v; want claude", f.Harness)
	}
	if f.Sync == nil || f.Sync.AllowRebase == nil || *f.Sync.AllowRebase {
		t.Errorf("sync.allow_rebase = %v; want false (preserved)", f.Sync)
	}
}
